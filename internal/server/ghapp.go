package server

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GitHubApp handles GitHub App authentication.
// It generates JWTs signed with the app's private key and exchanges them
// for short-lived installation tokens via the GitHub API.
type GitHubApp struct {
	appID          string
	installationID string
	privateKey     *rsa.PrivateKey

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// NewGitHubApp creates a new GitHub App authenticator.
func NewGitHubApp(appID, installationID, privateKeyPath string) (*GitHubApp, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	return &GitHubApp{
		appID:          appID,
		installationID: installationID,
		privateKey:     key,
	}, nil
}

// Token returns a valid installation access token, refreshing if needed.
func (a *GitHubApp) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Return cached token if still valid (with 1 minute buffer)
	if a.cachedToken != "" && time.Now().Add(time.Minute).Before(a.tokenExpiry) {
		return a.cachedToken, nil
	}

	// Generate JWT
	jwtToken, err := a.generateJWT()
	if err != nil {
		return "", fmt.Errorf("generating JWT: %w", err)
	}

	// Exchange JWT for installation token
	token, expiry, err := a.createInstallationToken(ctx, jwtToken)
	if err != nil {
		return "", fmt.Errorf("creating installation token: %w", err)
	}

	a.cachedToken = token
	a.tokenExpiry = expiry
	return token, nil
}

func (a *GitHubApp) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // clock skew tolerance
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),  // max 10 minutes
		Issuer:    a.appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(a.privateKey)
}

type installationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *GitHubApp) createInstallationToken(ctx context.Context, jwtToken string) (string, time.Time, error) {
	url := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", a.installationID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		return "", time.Time{}, fmt.Errorf("installation token API returned %d", resp.StatusCode)
	}

	var result installationTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, err
	}

	return result.Token, result.ExpiresAt, nil
}
