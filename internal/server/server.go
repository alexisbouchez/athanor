package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"context"
)

// Config holds server configuration.
type Config struct {
	ListenAddr    string // default ":8080"
	WebhookSecret string
	GitHubToken   string
	WorkspaceDir  string // default "/var/lib/athanor/workspaces"

	// VM configuration (empty KernelPath disables VM mode)
	KernelPath string
	RootfsPath string
	SSHKeyPath string
	VMDiskDir  string
	VMCPUs     int
	VMMemoryMB int
}

// UseVMs returns true if VM mode is configured.
func (c *Config) UseVMs() bool {
	return c.KernelPath != ""
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:    envOr("LISTEN_ADDR", ":8080"),
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		WorkspaceDir:  envOr("WORKSPACE_DIR", "/var/lib/athanor/workspaces"),
		KernelPath:    os.Getenv("KERNEL_PATH"),
		RootfsPath:    envOr("ROOTFS_PATH", "/var/lib/athanor/rootfs.ext4"),
		SSHKeyPath:    envOr("SSH_KEY_PATH", "/var/lib/athanor/vm-ssh-key"),
		VMDiskDir:     envOr("VM_DISK_DIR", "/var/lib/athanor/vm-disks"),
		VMCPUs:        envOrInt("VM_CPUS", 2),
		VMMemoryMB:    envOrInt("VM_MEMORY_MB", 2048),
	}
	if cfg.WebhookSecret == "" {
		return nil, fmt.Errorf("WEBHOOK_SECRET environment variable is required")
	}
	if cfg.GitHubToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	fmt.Sscanf(v, "%d", &n)
	if n <= 0 {
		return fallback
	}
	return n
}

// Server is the webhook HTTP server.
type Server struct {
	cfg    *Config
	worker *Worker
	gh     *GitHubClient
	logger *log.Logger
	mux    *http.ServeMux
}

// New creates a new server.
func New(cfg *Config) *Server {
	logger := log.New(os.Stdout, "[athanor] ", log.LstdFlags)
	gh := NewGitHubClient(cfg.GitHubToken)

	// Configure GitHub App if credentials are provided
	appID := os.Getenv("GITHUB_APP_ID")
	installationID := os.Getenv("GITHUB_APP_INSTALLATION_ID")
	appKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if appID != "" && installationID != "" && appKeyPath != "" {
		app, err := NewGitHubApp(appID, installationID, appKeyPath)
		if err != nil {
			logger.Printf("Warning: failed to configure GitHub App: %v", err)
		} else {
			gh.SetApp(app)
			logger.Printf("GitHub App configured (app_id=%s, installation_id=%s)", appID, installationID)
		}
	}

	store := NewRunStore(50)
	worker := NewWorker(cfg, gh, logger)
	worker.store = store

	s := &Server{
		cfg:    cfg,
		worker: worker,
		gh:     gh,
		logger: logger,
		mux:    http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /webhook", s.handleWebhook)
	s.mux.HandleFunc("GET /api/runs", s.handleAPIRuns)
	s.mux.HandleFunc("GET /api/events", s.handleSSE)
	s.mux.HandleFunc("GET /font/regular.woff2", s.handleFontRegular)
	s.mux.HandleFunc("GET /font/bold.woff2", s.handleFontBold)
	s.mux.HandleFunc("GET /", s.handleUI)

	return s
}

func (s *Server) handleAPIRuns(w http.ResponseWriter, r *http.Request) {
	runs := s.worker.store.Recent(20)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.worker.store.Subscribe()
	defer s.worker.store.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case run := <-ch:
			data, _ := json.Marshal(run)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleFontRegular(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Write(fontRegular)
}

func (s *Server) handleFontBold(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Write(fontBold)
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

// Run starts the server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// Start worker in background
	go s.worker.Start(ctx)

	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: s.mux,
	}

	// Graceful shutdown on context cancellation
	go func() {
		<-ctx.Done()
		s.logger.Println("Shutting down...")
		srv.Close()
	}()

	s.logger.Printf("Listening on %s", s.cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read body (limit 10MB)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Validate signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if !validateSignature(body, signature, s.cfg.WebhookSecret) {
		s.logger.Println("Invalid webhook signature")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	s.logger.Printf("Received event: %s", eventType)

	var job Job

	switch eventType {
	case "push":
		var event PushEvent
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		job = Job{
			RepoFullName: event.Repository.FullName,
			CloneURL:     event.Repository.CloneURL,
			SHA:          event.After,
			Ref:          event.Ref,
			EventName:    "push",
			Actor:        event.Sender.Login,
		}

	case "pull_request":
		var event PullRequestEvent
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		// Only handle opened, synchronize, reopened
		switch event.Action {
		case "opened", "synchronize", "reopened":
		default:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "ignoring action %q", event.Action)
			return
		}
		job = Job{
			RepoFullName: event.Repository.FullName,
			CloneURL:     event.Repository.CloneURL,
			SHA:          event.PullRequest.Head.SHA,
			Ref:          "refs/heads/" + event.PullRequest.Head.Ref,
			EventName:    "pull_request",
			Actor:        event.Sender.Login,
		}

	case "ping":
		s.logger.Println("Ping received - webhook configured correctly")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "pong")
		return

	default:
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ignoring event %q", eventType)
		return
	}

	if !s.worker.Enqueue(job) {
		s.logger.Println("Job queue full, rejecting")
		http.Error(w, "queue full", http.StatusServiceUnavailable)
		return
	}

	s.logger.Printf("Queued job for %s @ %s", job.RepoFullName, job.SHA[:8])
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, "queued")
}

func validateSignature(payload []byte, signature string, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
