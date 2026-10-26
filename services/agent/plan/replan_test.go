package plan

import (
	"context"
	"testing"

	"ai-meeting/clients"
)

func newKeyedPlan(tasks ...Task) *Plan {
	p := &Plan{Tasks: tasks}
	p.Normalize()
	if err := p.EnsureIdempotencyKeys(); err != nil {
		panic(err)
	}
	return p
}

func TestApplyDiffRejectsCompletedTask(t *testing.T) {
	p := newKeyedPlan(agentTask("t1"), agentTask("t2"))
	results := map[string]TaskResult{"t1": {ID: "t1", Status: StatusDone}}

	if _, _, err := p.ApplyDiff(PlanDiff{Remove: []string{"t1"}}, results); err == nil {
		t.Fatal("removing a completed task should fail")
	}
	if _, _, err := p.ApplyDiff(PlanDiff{Update: []Task{agentTask("t1")}}, results); err == nil {
		t.Fatal("updating a completed task should fail")
	}
}

func TestApplyDiffRejectsCompletedDescendant(t *testing.T) {
	p := newKeyedPlan(agentTask("t1"), agentTask("t2", "t1"))
	results := map[string]TaskResult{
		"t1": {ID: "t1", Status: StatusFailed},
		"t2": {ID: "t2", Status: StatusDone},
	}
	if _, _, err := p.ApplyDiff(PlanDiff{
		Update: []Task{{ID: "t1", Type: TaskTypeAgent, Goal: "changed"}},
	}, results); err == nil {
		t.Fatal("updating a task with a completed descendant should fail")
	}
}

func TestApplyDiffClearsDownstream(t *testing.T) {
	p := newKeyedPlan(agentTask("t1"), agentTask("t2", "t1"))
	results := map[string]TaskResult{
		"t1": {ID: "t1", Status: StatusFailed},
		"t2": {ID: "t2", Status: StatusFailed},
	}
	oldT2Key := p.Tasks[1].IdempotencyKey

	next, cleared, err := p.ApplyDiff(PlanDiff{
		Update: []Task{{ID: "t1", Type: TaskTypeAgent, Goal: "changed"}},
	}, results)
	if err != nil {
		t.Fatalf("apply diff: %v", err)
	}
	clearedSet := map[string]bool{}
	for _, id := range cleared {
		clearedSet[id] = true
	}
	if !clearedSet["t1"] || !clearedSet["t2"] {
		t.Fatalf("updated task and its descendant should be cleared, got %v", cleared)
	}
	if next.Tasks[1].IdempotencyKey == oldT2Key {
		t.Fatal("downstream idempotency key should change when upstream changes")
	}
}

func TestApplyDiffUpdatesPendingTask(t *testing.T) {
	p := newKeyedPlan(agentTask("t1"), agentTask("t2"))
	results := map[string]TaskResult{"t1": {ID: "t1", Status: StatusDone}, "t2": {ID: "t2", Status: StatusFailed}}
	oldKey := p.Tasks[1].IdempotencyKey

	next, cleared, err := p.ApplyDiff(PlanDiff{
		Reason: "change approach",
		Update: []Task{{ID: "t2", Type: TaskTypeAgent, Goal: "t2-new"}},
	}, results)
	if err != nil {
		t.Fatalf("apply diff: %v", err)
	}
	if next.Tasks[1].Goal != "t2-new" || next.Tasks[1].IdempotencyKey == oldKey {
		t.Fatalf("updated task should have a new key: %+v", next.Tasks[1])
	}
	if len(cleared) != 1 || cleared[0] != "t2" {
		t.Fatalf("updated task should be cleared for re-execution, got %v", cleared)
	}
	if err := next.Validate(6, nil); err != nil {
		t.Fatalf("updated plan should be valid: %v", err)
	}
}

func TestApplyDiffReplacesFailedTask(t *testing.T) {
	p := newKeyedPlan(agentTask("t1"), agentTask("t2"))
	results := map[string]TaskResult{"t1": {ID: "t1", Status: StatusFailed}}

	next, cleared, err := p.ApplyDiff(PlanDiff{
		Reason: "replace failed task",
		Remove: []string{"t1"},
		Add: []Task{{
			ID: "t1b", Type: TaskTypeAgent, Goal: "retry with a different approach",
			Replaces: "t1",
		}},
	}, results)
	if err != nil {
		t.Fatalf("apply diff: %v", err)
	}
	if next.Tasks[0].ID != "t2" || next.Tasks[1].ID != "t1b" {
		t.Fatalf("unexpected task order: %+v", next.Tasks)
	}
	if len(cleared) != 1 || cleared[0] != "t1" {
		t.Fatalf("removed task should be cleared, got %v", cleared)
	}
	if next.Tasks[1].IdempotencyKey == "" || next.Tasks[1].Replaces != "t1" {
		t.Fatalf("replacement should have key and replaces: %+v", next.Tasks[1])
	}
}

func TestRunRoundStopsForReplanAndResumes(t *testing.T) {
	p := newKeyedPlan(agentTask("t1"), agentTask("t2"))
	s := &Scheduler{MaxParallel: 1, ReplanEnabled: true, Ledger: NewLedger()}

	runner := func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
		if task.ID == "t1" {
			return TaskResult{ID: task.ID, Status: StatusFailed, Error: "no data"}
		}
		return TaskResult{ID: task.ID, Status: StatusDone, Summary: "ok"}
	}

	rr := s.RunRound(context.Background(), p, nil, runner)
	if !rr.NeedsReplan || len(rr.Reasons) == 0 {
		t.Fatalf("failure should trigger replan, got %+v", rr)
	}
	if rr.Results["t1"].Status != StatusFailed || rr.Results["t2"].Status != StatusDone {
		t.Fatalf("unexpected first round results: %+v", rr.Results)
	}

	next, cleared, err := p.ApplyDiff(PlanDiff{
		Reason: "replace t1",
		Remove: []string{"t1"},
		Add:    []Task{{ID: "t1b", Type: TaskTypeAgent, Goal: "try another source", Replaces: "t1"}},
	}, rr.Results)
	if err != nil {
		t.Fatalf("apply diff: %v", err)
	}
	if err := next.Validate(6, nil); err != nil {
		t.Fatalf("validate next plan: %v", err)
	}
	for _, id := range cleared {
		delete(rr.Results, id)
	}

	rr2 := s.RunRound(context.Background(), next, rr.Results, func(ctx context.Context, task Task, upstream map[string]TaskResult) TaskResult {
		return TaskResult{ID: task.ID, Status: StatusDone, Summary: "ok"}
	})
	if rr2.NeedsReplan {
		t.Fatalf("successful resume should not need replan: %+v", rr2)
	}
	if rr2.Results["t1b"].Status != StatusDone || rr2.Results["t2"].Status != StatusDone {
		t.Fatalf("unexpected second round results: %+v", rr2.Results)
	}
	if _, exists := rr2.Results["t1"]; exists {
		t.Fatal("removed task result should be cleared")
	}
}

func TestReplannerParsesDiff(t *testing.T) {
	orig := callReplannerLLM
	callReplannerLLM = func(ctx context.Context, aiID uint, messages []clients.PromptMessage, temperature float64) (string, error) {
		return "```json\n" + `{"reason":"need another source","add":[{"id":"t2","type":"agent","goal":"search another source","depends_on":[]}],"remove":["t1"]}` + "\n```", nil
	}
	t.Cleanup(func() { callReplannerLLM = orig })

	p := newKeyedPlan(agentTask("t1"))
	diff, err := NewLLMReplanner().Propose(context.Background(), ReplanInput{
		Goal:        "find data",
		Plan:        p,
		Results:     map[string]TaskResult{"t1": {ID: "t1", Status: StatusFailed, Error: "no data"}},
		Reasons:     []string{"t1 failed"},
		MaxNewTasks: 2,
		AgentID:     1,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if diff.Reason != "need another source" || len(diff.Add) != 1 || diff.Add[0].ID != "t2" {
		t.Fatalf("unexpected diff: %+v", diff)
	}
	if diff.Add[0].IdempotencyKey != "" {
		t.Fatal("model-supplied idempotency key must be cleared")
	}
}

func TestReplannerNoChange(t *testing.T) {
	orig := callReplannerLLM
	callReplannerLLM = func(ctx context.Context, aiID uint, messages []clients.PromptMessage, temperature float64) (string, error) {
		return `{"reason":"no_change"}`, nil
	}
	t.Cleanup(func() { callReplannerLLM = orig })

	p := newKeyedPlan(agentTask("t1"))
	diff, err := NewLLMReplanner().Propose(context.Background(), ReplanInput{
		Goal: "find data", Plan: p,
		Results: map[string]TaskResult{"t1": {ID: "t1", Status: StatusFailed}},
		Reasons: []string{"t1 failed"}, MaxNewTasks: 2,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if diff.Reason != "no_change" {
		t.Fatalf("want no_change, got %+v", diff)
	}
}

func TestReplannerAcceptsStringAcceptance(t *testing.T) {
	orig := callReplannerLLM
	callReplannerLLM = func(ctx context.Context, aiID uint, messages []clients.PromptMessage, temperature float64) (string, error) {
		return `{"reason":"add fallback","add":[{"id":"t2","type":"llm","goal":"给结论","acceptance":"给出结论"}]}`, nil
	}
	t.Cleanup(func() { callReplannerLLM = orig })

	p := newKeyedPlan(agentTask("t1"))
	diff, err := NewLLMReplanner().Propose(context.Background(), ReplanInput{
		Goal: "find data", Plan: p,
		Results: map[string]TaskResult{"t1": {ID: "t1", Status: StatusFailed}},
		Reasons: []string{"t1 failed"}, MaxNewTasks: 2,
	})
	if err != nil {
		t.Fatalf("string acceptance should be accepted: %v", err)
	}
	if len(diff.Add) != 1 || len(diff.Add[0].Acceptance) != 1 || diff.Add[0].Acceptance[0] != "给出结论" {
		t.Fatalf("unexpected acceptance: %+v", diff.Add)
	}
}

func TestPlanHashDetectsChanges(t *testing.T) {
	p1 := newKeyedPlan(agentTask("t1"), agentTask("t2", "t1"))
	p2 := newKeyedPlan(agentTask("t1"), agentTask("t2", "t1"))
	if planHash(p1) != planHash(p2) {
		t.Fatal("same plan should have the same hash")
	}
	p3 := newKeyedPlan(agentTask("t1"), Task{ID: "t2", Type: TaskTypeAgent, Goal: "changed", DependsOn: []string{"t1"}})
	if planHash(p1) == planHash(p3) {
		t.Fatal("changed plan should have a different hash")
	}
	if planHash(p1) == "" {
		t.Fatal("plan hash should not be empty")
	}
}
