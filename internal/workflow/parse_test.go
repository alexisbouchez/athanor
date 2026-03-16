package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSimple(t *testing.T) {
	w, err := ParseFile("testdata/simple.yml")
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "Simple Test" {
		t.Errorf("name = %q, want %q", w.Name, "Simple Test")
	}
	if len(w.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(w.Jobs))
	}
	job := w.Jobs["build"]
	if len(job.Steps) != 2 {
		t.Errorf("steps = %d, want 2", len(job.Steps))
	}
}

func TestParseDAG(t *testing.T) {
	w, err := ParseFile("testdata/dag.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Jobs) != 4 {
		t.Fatalf("jobs = %d, want 4", len(w.Jobs))
	}
	if len(w.On.Events) != 2 {
		t.Errorf("events = %d, want 2", len(w.On.Events))
	}
	deploy := w.Jobs["deploy"]
	if len(deploy.Needs) != 2 {
		t.Errorf("deploy needs = %d, want 2", len(deploy.Needs))
	}
}

func TestParseEnvOutputs(t *testing.T) {
	w, err := ParseFile("testdata/env-outputs.yml")
	if err != nil {
		t.Fatal(err)
	}
	if w.Env["GLOBAL_VAR"] != "hello" {
		t.Errorf("global env = %q, want %q", w.Env["GLOBAL_VAR"], "hello")
	}
	producer := w.Jobs["producer"]
	if producer.Env["JOB_VAR"] != "world" {
		t.Errorf("job env = %q, want %q", producer.Env["JOB_VAR"], "world")
	}
	if producer.Steps[0].ID != "greet" {
		t.Errorf("step id = %q, want %q", producer.Steps[0].ID, "greet")
	}
}

func TestParseConditions(t *testing.T) {
	w, err := ParseFile("testdata/conditions.yml")
	if err != nil {
		t.Fatal(err)
	}
	steps := w.Jobs["main"].Steps
	if steps[0].If != "always()" {
		t.Errorf("step 0 if = %q, want %q", steps[0].If, "always()")
	}
	if !steps[2].ContinueOnError {
		t.Error("step 2 continue-on-error should be true")
	}
}

func TestParseDefaults(t *testing.T) {
	w, err := ParseFile("testdata/defaults.yml")
	if err != nil {
		t.Fatal(err)
	}
	job := w.Jobs["build"]
	if job.Defaults.Run.Shell != "bash" {
		t.Errorf("default shell = %q, want %q", job.Defaults.Run.Shell, "bash")
	}
	if job.Defaults.Run.WorkingDirectory != "./src" {
		t.Errorf("default working-directory = %q, want %q", job.Defaults.Run.WorkingDirectory, "./src")
	}
}

func TestParseInvalidNeeds(t *testing.T) {
	_, err := ParseFile("testdata/invalid-needs.yml")
	if err == nil {
		t.Fatal("expected error for invalid needs reference")
	}
}

func TestDiscoverWorkflows(t *testing.T) {
	// Use a subdirectory with only valid workflows
	dir := t.TempDir()
	for _, name := range []string{"simple.yml", "dag.yml", "defaults.yml", "conditions.yml"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	workflows, err := DiscoverWorkflows(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) != 4 {
		t.Errorf("discovered %d workflows, want 4", len(workflows))
	}
	for _, w := range workflows {
		if w.Path == "" {
			t.Errorf("workflow path should be set")
		}
	}
}

func TestParseUsesAndMatrix(t *testing.T) {
	w, err := ParseFile("testdata/uses-matrix.yml")
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "Uses and Matrix" {
		t.Errorf("name = %q", w.Name)
	}
	job := w.Jobs["test"]
	if len(job.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(job.Steps))
	}

	// Step 0: uses only
	if job.Steps[0].Uses != "actions/checkout@v4" {
		t.Errorf("step 0 uses = %q", job.Steps[0].Uses)
	}
	if job.Steps[0].Run != "" {
		t.Errorf("step 0 run should be empty")
	}

	// Step 1: uses with with:
	if job.Steps[1].Uses != "actions/setup-go@v5" {
		t.Errorf("step 1 uses = %q", job.Steps[1].Uses)
	}
	if job.Steps[1].With["go-version"] != "${{ matrix.go }}" {
		t.Errorf("step 1 with.go-version = %q", job.Steps[1].With["go-version"])
	}

	// Step 2: run step with timeout
	if job.Steps[2].TimeoutMinutes != 10 {
		t.Errorf("step 2 timeout = %d, want 10", job.Steps[2].TimeoutMinutes)
	}

	// Matrix
	if len(job.Strategy.Matrix.Values) != 2 {
		t.Fatalf("matrix values = %d, want 2", len(job.Strategy.Matrix.Values))
	}
	if len(job.Strategy.Matrix.Values["os"]) != 2 {
		t.Errorf("matrix os = %d, want 2", len(job.Strategy.Matrix.Values["os"]))
	}
	if len(job.Strategy.Matrix.Exclude) != 1 {
		t.Errorf("matrix exclude = %d, want 1", len(job.Strategy.Matrix.Exclude))
	}
	if len(job.Strategy.Matrix.Include) != 1 {
		t.Errorf("matrix include = %d, want 1", len(job.Strategy.Matrix.Include))
	}
	if job.Strategy.FailFast != nil && *job.Strategy.FailFast != false {
		t.Errorf("fail-fast = %v, want false", *job.Strategy.FailFast)
	}
	if job.Strategy.MaxParallel != 2 {
		t.Errorf("max-parallel = %d, want 2", job.Strategy.MaxParallel)
	}

	// Job timeout and outputs
	if job.TimeoutMinutes != 30 {
		t.Errorf("job timeout = %d, want 30", job.TimeoutMinutes)
	}
	if job.Outputs["result"] != "${{ steps.build.outputs.result }}" {
		t.Errorf("job outputs.result = %q", job.Outputs["result"])
	}
}

func TestParseRunXorUses(t *testing.T) {
	// Both run and uses should fail
	data := []byte(`
name: Invalid
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
        uses: actions/checkout@v4
`)
	_, err := ParseBytes(data)
	if err == nil {
		t.Fatal("expected error for step with both run: and uses:")
	}

	// Neither run nor uses should fail
	data = []byte(`
name: Invalid
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: nothing
`)
	_, err = ParseBytes(data)
	if err == nil {
		t.Fatal("expected error for step with neither run: nor uses:")
	}
}

func TestParseUsesWithShellFails(t *testing.T) {
	data := []byte(`
name: Invalid
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        shell: bash
`)
	_, err := ParseBytes(data)
	if err == nil {
		t.Fatal("expected error for uses: step with shell:")
	}
}

func TestStringOnMap(t *testing.T) {
	data := []byte(`
name: Map Trigger
on:
  push:
    branches: [main]
  pull_request:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)
	w, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.On.Events) != 2 {
		t.Errorf("events = %v, want 2 events", w.On.Events)
	}
}
