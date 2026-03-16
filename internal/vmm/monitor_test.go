package vmm

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetVMInfo(t *testing.T) {
	// Set up a Unix socket server that mimics cloud-hypervisor's API.
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")

	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	want := VMInfo{
		State: "Running",
		Config: &VMConfig{
			CPUs:   &CPUsConfig{BootVCPUs: 2, MaxVCPUs: 4},
			Memory: &MemoryConfig{Size: 512 * 1024 * 1024},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vm.info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Close()

	monitor := NewMonitor(sock, time.Second)
	ctx := context.Background()

	got, err := monitor.GetVMInfo(ctx)
	if err != nil {
		t.Fatalf("GetVMInfo: %v", err)
	}

	if got.State != want.State {
		t.Errorf("state = %q, want %q", got.State, want.State)
	}
	if got.Config.CPUs.BootVCPUs != want.Config.CPUs.BootVCPUs {
		t.Errorf("boot_vcpus = %d, want %d", got.Config.CPUs.BootVCPUs, want.Config.CPUs.BootVCPUs)
	}
	if got.Config.Memory.Size != want.Config.Memory.Size {
		t.Errorf("memory = %d, want %d", got.Config.Memory.Size, want.Config.Memory.Size)
	}
}

func TestGetVMInfoSocketMissing(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nonexistent.sock")
	monitor := NewMonitor(sock, time.Second)

	_, err := monitor.GetVMInfo(context.Background())
	if err == nil {
		t.Fatal("expected error for missing socket")
	}
}

func TestMonitorRunCancellation(t *testing.T) {
	// Verify Run exits cleanly when context is cancelled.
	sock := filepath.Join(t.TempDir(), "unused.sock")
	monitor := NewMonitor(sock, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	if err := monitor.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
