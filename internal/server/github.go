package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Webhook payload types — only the fields we need.

type PushEvent struct {
	Ref        string     `json:"ref"`
	After      string     `json:"after"`
	Repository Repository `json:"repository"`
	Sender     Sender     `json:"sender"`
}

type PullRequestEvent struct {
	Action      string      `json:"action"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  Repository  `json:"repository"`
	Sender      Sender      `json:"sender"`
}

type Repository struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
}

type PullRequest struct {
	Head PRRef `json:"head"`
}

type PRRef struct {
	SHA string `json:"sha"`
	Ref string `json:"ref"`
}

type Sender struct {
	Login string `json:"login"`
}

// GitHubClient talks to the GitHub API (Checks API + commit statuses).
type GitHubClient struct {
	token      string
	httpClient *http.Client
}

// NewGitHubClient creates a new GitHub API client.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		token:      token,
		httpClient: &http.Client{},
	}
}

// --- Checks API (standard way to report CI with logs) ---

// CheckRun is the request/response for the GitHub Checks API.
type CheckRun struct {
	ID          int64          `json:"id,omitempty"`
	Name        string         `json:"name"`
	HeadSHA     string         `json:"head_sha"`
	Status      string         `json:"status,omitempty"`       // "queued", "in_progress", "completed"
	Conclusion  string         `json:"conclusion,omitempty"`   // "success", "failure", "cancelled", "skipped"
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Output      *CheckOutput   `json:"output,omitempty"`
}

// CheckOutput is the output section of a check run.
type CheckOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text,omitempty"` // Markdown, up to 65535 chars
}

// CreateCheckRun creates a new check run via POST /repos/{owner}/{repo}/check-runs.
func (c *GitHubClient) CreateCheckRun(ctx context.Context, repo string, cr CheckRun) (int64, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/check-runs", repo)

	var result struct {
		ID int64 `json:"id"`
	}
	if err := c.apiPost(ctx, url, cr, &result); err != nil {
		return 0, fmt.Errorf("creating check run: %w", err)
	}
	return result.ID, nil
}

// UpdateCheckRun updates an existing check run via PATCH /repos/{owner}/{repo}/check-runs/{id}.
func (c *GitHubClient) UpdateCheckRun(ctx context.Context, repo string, checkRunID int64, cr CheckRun) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/check-runs/%d", repo, checkRunID)
	return c.apiPatch(ctx, url, cr)
}

// --- Legacy commit status (fallback) ---

type commitStatus struct {
	State       string `json:"state"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

// SetCommitStatus creates a commit status on the given SHA.
func (c *GitHubClient) SetCommitStatus(ctx context.Context, repo, sha, state, description, statusContext string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/statuses/%s", repo, sha)

	body := commitStatus{
		State:       state,
		Description: truncate(description, 140),
		Context:     statusContext,
	}
	return c.apiPost(ctx, url, body, nil)
}

// --- HTTP helpers ---

func (c *GitHubClient) apiPost(ctx context.Context, url string, body any, result any) error {
	return c.apiDo(ctx, "POST", url, body, result)
}

func (c *GitHubClient) apiPatch(ctx context.Context, url string, body any) error {
	return c.apiDo(ctx, "PATCH", url, body, nil)
}

func (c *GitHubClient) apiDo(ctx context.Context, method, url string, body any, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("github API %s %s returned %d: %s", method, url, resp.StatusCode, truncate(string(respBody), 200))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
