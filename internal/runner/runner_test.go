package runner

import (
	"context"
	"sort"
	"testing"

	"github.com/alexj212/athanor/internal/workflow"
)

func mockExec(exitCode int) ExecStepFunc {
	return func(ctx context.Context, script string, opts ExecOptions, lines chan<- string) (*StepResult, error) {
		if lines != nil {
			lines <- "mock output"
		}
		return &StepResult{ExitCode: exitCode}, nil
	}
}

func collectEvents(r *Runner) []RunEvent {
	var events []RunEvent
	for e := range r.Events() {
		events = append(events, e)
	}
	return events
}

func TestDAGOrdering(t *testing.T) {
	wf := &workflow.Workflow{
		Name: "DAG Test",
		Jobs: map[string]workflow.Job{
			"lint": {Steps: []workflow.Step{{Run: "echo lint"}}},
			"test": {Needs: workflow.StringOrSlice{"lint"}, Steps: []workflow.Step{{Run: "echo test"}}},
			"build": {Needs: workflow.StringOrSlice{"lint"}, Steps: []workflow.Step{{Run: "echo build"}}},
			"deploy": {Needs: workflow.StringOrSlice{"test", "build"}, Steps: []workflow.Step{{Run: "echo deploy"}}},
		},
	}

	r := NewRunnerWithExec(wf, mockExec(0))
	go r.Run(context.Background())

	var jobOrder []string
	for e := range r.Events() {
		if js, ok := e.(JobStarted); ok {
			jobOrder = append(jobOrder, js.JobID)
		}
	}

	// lint must come before test and build, both must come before deploy
	indexOf := func(s string) int {
		for i, v := range jobOrder {
			if v == s {
				return i
			}
		}
		return -1
	}
	if indexOf("lint") >= indexOf("test") || indexOf("lint") >= indexOf("build") {
		t.Errorf("lint should run before test and build, got order: %v", jobOrder)
	}
	if indexOf("test") >= indexOf("deploy") || indexOf("build") >= indexOf("deploy") {
		t.Errorf("test and build should run before deploy, got order: %v", jobOrder)
	}
}

func TestFailurePropagation(t *testing.T) {
	wf := &workflow.Workflow{
		Name: "Failure Test",
		Jobs: map[string]workflow.Job{
			"first":  {Steps: []workflow.Step{{Run: "exit 1"}}},
			"second": {Needs: workflow.StringOrSlice{"first"}, Steps: []workflow.Step{{Run: "echo ok"}}},
		},
	}

	r := NewRunnerWithExec(wf, mockExec(1))
	go r.Run(context.Background())

	events := collectEvents(r)

	var finishes []JobFinished
	for _, e := range events {
		if jf, ok := e.(JobFinished); ok {
			finishes = append(finishes, jf)
		}
	}

	sort.Slice(finishes, func(i, j int) bool { return finishes[i].JobID < finishes[j].JobID })

	if finishes[0].Status != "failure" {
		t.Errorf("first job should fail, got %s", finishes[0].Status)
	}
	if finishes[1].Status != "skipped" {
		t.Errorf("second job should be skipped, got %s", finishes[1].Status)
	}
}

func TestContinueOnError(t *testing.T) {
	wf := &workflow.Workflow{
		Name: "Continue Test",
		Jobs: map[string]workflow.Job{
			"test": {Steps: []workflow.Step{
				{Run: "exit 1", ContinueOnError: true},
				{Run: "echo ok"},
			}},
		},
	}

	// First step fails, but continue-on-error is set
	callCount := 0
	exec := func(ctx context.Context, script string, opts ExecOptions, lines chan<- string) (*StepResult, error) {
		callCount++
		if callCount == 1 {
			return &StepResult{ExitCode: 1}, nil
		}
		return &StepResult{ExitCode: 0}, nil
	}

	r := NewRunnerWithExec(wf, exec)
	go r.Run(context.Background())
	collectEvents(r)

	if callCount != 2 {
		t.Errorf("expected 2 steps to execute, got %d", callCount)
	}
}

func TestConditionEvaluation(t *testing.T) {
	tests := []struct {
		condition string
		failed    bool
		want      bool
	}{
		{"", false, true},
		{"", true, false},
		{"success()", false, true},
		{"success()", true, false},
		{"failure()", false, false},
		{"failure()", true, true},
		{"always()", false, true},
		{"always()", true, true},
	}
	for _, tt := range tests {
		ctx := map[string]any{}
		if tt.failed {
			ctx["__job_status"] = "failure"
		} else {
			ctx["__job_status"] = "success"
		}
		got := shouldRun(tt.condition, tt.failed, ctx)
		if got != tt.want {
			t.Errorf("shouldRun(%q, %v) = %v, want %v", tt.condition, tt.failed, got, tt.want)
		}
	}
}

func TestTopoSortCycle(t *testing.T) {
	jobs := map[string]workflow.Job{
		"a": {Needs: workflow.StringOrSlice{"b"}, Steps: []workflow.Step{{Run: "echo a"}}},
		"b": {Needs: workflow.StringOrSlice{"a"}, Steps: []workflow.Step{{Run: "echo b"}}},
	}
	_, err := topoSort(jobs)
	if err == nil {
		t.Error("expected cycle error")
	}
}

func TestWorkflowFinishStatus(t *testing.T) {
	wf := &workflow.Workflow{
		Name: "Success Test",
		Jobs: map[string]workflow.Job{
			"test": {Steps: []workflow.Step{{Run: "echo ok"}}},
		},
	}

	r := NewRunnerWithExec(wf, mockExec(0))
	go r.Run(context.Background())
	events := collectEvents(r)

	last := events[len(events)-1]
	wf2, ok := last.(WorkflowFinished)
	if !ok {
		t.Fatalf("last event should be WorkflowFinished, got %T", last)
	}
	if wf2.Status != "success" {
		t.Errorf("workflow status = %q, want %q", wf2.Status, "success")
	}
}
