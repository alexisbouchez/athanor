package server

import (
	"context"
	"sync"
)

// ConcurrencyManager enforces concurrency groups.
// Only one run per group can be active at a time.
// If cancel-in-progress is set, a new run cancels the existing one.
type ConcurrencyManager struct {
	mu     sync.Mutex
	groups map[string]*concurrencyEntry
}

type concurrencyEntry struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewConcurrencyManager creates a new concurrency manager.
func NewConcurrencyManager() *ConcurrencyManager {
	return &ConcurrencyManager{
		groups: make(map[string]*concurrencyEntry),
	}
}

// Acquire acquires a slot in the given concurrency group.
// If cancelInProgress is true and another run is active, it cancels it.
// Otherwise, it waits for the existing run to complete.
// Returns a context derived from the parent and a release function.
func (cm *ConcurrencyManager) Acquire(ctx context.Context, group string, cancelInProgress bool) (context.Context, func()) {
	cm.mu.Lock()

	if existing, ok := cm.groups[group]; ok {
		if cancelInProgress {
			// Cancel the existing run
			existing.cancel()
			done := existing.done
			cm.mu.Unlock()
			// Wait for it to finish
			select {
			case <-done:
			case <-ctx.Done():
				return ctx, func() {}
			}
			cm.mu.Lock()
		} else {
			done := existing.done
			cm.mu.Unlock()
			// Wait for it to finish
			select {
			case <-done:
			case <-ctx.Done():
				return ctx, func() {}
			}
			cm.mu.Lock()
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	cm.groups[group] = &concurrencyEntry{cancel: cancel, done: done}
	cm.mu.Unlock()

	release := func() {
		cancel()
		close(done)
		cm.mu.Lock()
		if entry, ok := cm.groups[group]; ok && entry.done == done {
			delete(cm.groups, group)
		}
		cm.mu.Unlock()
	}

	return runCtx, release
}
