package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// GitHubClient posts commit statuses to the GitHub API.
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

// commitStatus is the request body for the GitHub commit status API.
type commitStatus struct {
	State       string `json:"state"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

// SetCommitStatus creates a commit status on the given SHA.
// state: "pending", "success", "failure", "error"
func (c *GitHubClient) SetCommitStatus(ctx context.Context, repo, sha, state, description, statusContext string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/statuses/%s", repo, sha)

	body := commitStatus{
		State:       state,
		Description: truncate(description, 140),
		Context:     statusContext,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("setting commit status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("github status API returned %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
