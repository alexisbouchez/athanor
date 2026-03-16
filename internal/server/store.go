package server

import (
	"sync"
	"time"
)

// RunStore keeps recent build runs in memory.
type RunStore struct {
	mu   sync.RWMutex
	runs []*Run
	max  int

	// SSE subscribers
	subMu sync.Mutex
	subs  map[chan SSEEvent]struct{}
}

// SSEEvent is a typed event for SSE streaming.
type SSEEvent struct {
	Type string `json:"type"` // "run", "job", "step", "log", "done"
	Run  string `json:"run"`
	Data any    `json:"data"`
}

// Run represents a single CI run.
type Run struct {
	ID        string        `json:"id"`
	Repo      string        `json:"repo"`
	SHA       string        `json:"sha"`
	Ref       string        `json:"ref"`
	Actor     string        `json:"actor"`
	Event     string        `json:"event"`
	Status    string        `json:"status"`
	StartedAt time.Time     `json:"started_at"`
	Duration  float64       `json:"duration_secs,omitempty"`
	Workflows []WorkflowRun `json:"workflows"`
}

// WorkflowRun represents a workflow within a run.
type WorkflowRun struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Duration  float64   `json:"duration_secs,omitempty"`
	Jobs      []JobRun  `json:"jobs"`
}

// JobRun represents a job within a workflow run.
type JobRun struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Duration  float64   `json:"duration_secs,omitempty"`
	Steps     []StepRun `json:"steps"`
}

// StepRun represents a step within a job run.
type StepRun struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Duration  float64   `json:"duration_secs,omitempty"`
	Lines     []LogLine `json:"lines,omitempty"`
}

// LogLine is a single log line with a timestamp.
type LogLine struct {
	Time time.Time `json:"t"`
	Text string    `json:"s"`
}

// NewRunStore creates a store that keeps the last N runs.
func NewRunStore(max int) *RunStore {
	return &RunStore{
		max:  max,
		subs: make(map[chan SSEEvent]struct{}),
	}
}

// Add adds a new run and notifies subscribers.
func (s *RunStore) Add(r *Run) {
	s.mu.Lock()
	s.runs = append([]*Run{r}, s.runs...)
	if len(s.runs) > s.max {
		s.runs = s.runs[:s.max]
	}
	s.mu.Unlock()
	s.Emit(SSEEvent{Type: "run", Run: r.ID, Data: r})
}

// Update updates the first run matching the ID.
func (s *RunStore) Update(id string, fn func(*Run)) {
	s.mu.Lock()
	for _, r := range s.runs {
		if r.ID == id {
			fn(r)
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()
}

// Emit sends a typed event to all SSE subscribers.
func (s *RunStore) Emit(e SSEEvent) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Recent returns the last N runs.
func (s *RunStore) Recent(n int) []*Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n > len(s.runs) {
		n = len(s.runs)
	}
	out := make([]*Run, n)
	copy(out, s.runs[:n])
	return out
}

// Subscribe returns a channel that receives SSE events.
func (s *RunStore) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber.
func (s *RunStore) Unsubscribe(ch chan SSEEvent) {
	s.subMu.Lock()
	delete(s.subs, ch)
	s.subMu.Unlock()
	close(ch)
}
