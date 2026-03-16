package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// SecretStore manages per-repo secrets, persisted to disk as JSON.
// File format: {"owner/repo": {"KEY": "value", ...}, ...}
type SecretStore struct {
	mu   sync.RWMutex
	path string
	data map[string]map[string]string // repo -> key -> value
}

// NewSecretStore loads secrets from the given file path.
// Creates the file if it doesn't exist.
func NewSecretStore(path string) (*SecretStore, error) {
	s := &SecretStore{
		path: path,
		data: make(map[string]map[string]string),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err == nil && len(raw) > 0 {
		json.Unmarshal(raw, &s.data)
	}
	return s, nil
}

// Get returns all secrets for a repo.
func (s *SecretStore) Get(repo string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.data[repo]
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ListRepos returns all repos that have secrets configured.
func (s *SecretStore) ListRepos() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	repos := make([]string, 0, len(s.data))
	for repo := range s.data {
		repos = append(repos, repo)
	}
	return repos
}

// Set sets a single secret for a repo and persists to disk.
func (s *SecretStore) Set(repo, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[repo] == nil {
		s.data[repo] = make(map[string]string)
	}
	s.data[repo][key] = value
	return s.save()
}

// SetAll replaces all secrets for a repo and persists to disk.
func (s *SecretStore) SetAll(repo string, secrets map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(secrets) == 0 {
		delete(s.data, repo)
	} else {
		s.data[repo] = secrets
	}
	return s.save()
}

// Delete removes a single secret for a repo and persists to disk.
func (s *SecretStore) Delete(repo, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.data[repo]; ok {
		delete(m, key)
		if len(m) == 0 {
			delete(s.data, repo)
		}
	}
	return s.save()
}

// Merge returns global secrets merged with repo-specific secrets.
// Repo secrets take precedence over global secrets.
func (s *SecretStore) Merge(global map[string]string, repo string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	merged := make(map[string]string, len(global))
	for k, v := range global {
		merged[k] = v
	}
	for k, v := range s.data[repo] {
		merged[k] = v
	}
	return merged
}

// Keys returns secret key names for a repo (values masked).
func (s *SecretStore) Keys(repo string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.data[repo]
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (s *SecretStore) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}
