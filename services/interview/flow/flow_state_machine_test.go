package flow

import (
	"context"
	"testing"

	"ai-meeting/models"
	"ai-meeting/services/interview/runtime"
)

func newTestStateMachine(t *testing.T) *FlowStateMachine {
	t.Helper()
	return NewFlowStateMachine(runtime.NewFlowCache(newTestRedis(t)))
}

func TestFlowStateMachine_EnsureInitialized(t *testing.T) {
	fsm := newTestStateMachine(t)
	ctx := context.Background()

	state, err := fsm.EnsureInitialized(ctx, "s1", 3)
	if err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}
	if state.Status != models.FlowAsking {
		t.Errorf("initial status = %s, want ASKING", state.Status)
	}
	if state.CurrentQuestionNumber != "1" || state.TotalQuestions != 3 || state.Version != 1 {
		t.Errorf("initial state mismatch: %+v", state)
	}

	// 幂等: 再次初始化返回已有状态, 不覆盖
	again, err := fsm.EnsureInitialized(ctx, "s1", 99)
	if err != nil {
		t.Fatalf("EnsureInitialized again failed: %v", err)
	}
	if again.TotalQuestions != 3 {
		t.Errorf("EnsureInitialized overwrote existing flow: %+v", again)
	}
}

func TestFlowStateMachine_Transitions(t *testing.T) {
	fsm := newTestStateMachine(t)
	ctx := context.Background()

	if _, err := fsm.EnsureInitialized(ctx, "s1", 3); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}

	// 第 1 题: ASKING → EVALUATING(评分) → 推进到第 2 题 ASKING
	st, err := fsm.MoveToEvaluating(ctx, "s1")
	if err != nil {
		t.Fatalf("MoveToEvaluating failed: %v", err)
	}
	if st.Status != models.FlowEvaluating {
		t.Errorf("status = %s, want EVALUATING", st.Status)
	}
	st, err = fsm.AdvanceMainQuestion(ctx, "s1")
	if err != nil {
		t.Fatalf("AdvanceMainQuestion failed: %v", err)
	}
	if st.Status != models.FlowAsking || st.CurrentIndex != 1 || st.CurrentQuestionNumber != "2" {
		t.Errorf("after advance: %+v", st)
	}

	// 第 2 题: ASKING → FOLLOW_UP(追问)
	st, err = fsm.StartFollowUpQuestion(ctx, "s1", "2-F1")
	if err != nil {
		t.Fatalf("StartFollowUpQuestion failed: %v", err)
	}
	if st.Status != models.FlowFollowUp || st.FollowUpCount != 1 || st.CurrentQuestionNumber != "2-F1" {
		t.Errorf("after follow up: %+v", st)
	}

	// 追问结束: FOLLOW_UP → EVALUATING → 推进到第 3 题 ASKING
	st, err = fsm.MoveToEvaluating(ctx, "s1")
	if err != nil {
		t.Fatalf("MoveToEvaluating from FOLLOW_UP failed: %v", err)
	}
	if st.Status != models.FlowEvaluating {
		t.Errorf("status = %s, want EVALUATING", st.Status)
	}
	st, err = fsm.AdvanceMainQuestion(ctx, "s1")
	if err != nil {
		t.Fatalf("AdvanceMainQuestion failed: %v", err)
	}
	if st.Status != models.FlowAsking || st.CurrentIndex != 2 || st.CurrentQuestionNumber != "3" {
		t.Errorf("after advance to last: %+v", st)
	}

	// 第 3 题(最后一题): EVALUATING → 推进 → COMPLETED
	st, err = fsm.MoveToEvaluating(ctx, "s1")
	if err != nil {
		t.Fatalf("MoveToEvaluating failed: %v", err)
	}
	st, err = fsm.AdvanceMainQuestion(ctx, "s1")
	if err != nil {
		t.Fatalf("AdvanceMainQuestion to end failed: %v", err)
	}
	if st.Status != models.FlowCompleted || !st.IsCompleted() {
		t.Errorf("status = %s, want COMPLETED", st.Status)
	}
}

func TestFlowStateMachine_IllegalTransition(t *testing.T) {
	fsm := newTestStateMachine(t)
	ctx := context.Background()

	// 直接构造终态: 终态不可再转移
	if err := fsm.flowCache.SaveFlow(ctx, "s1", &models.InterviewFlowState{
		Status:                models.FlowCompleted,
		CurrentIndex:          3,
		TotalQuestions:        3,
		CurrentQuestionNumber: "",
		Version:               1,
	}); err != nil {
		t.Fatalf("SaveFlow failed: %v", err)
	}

	if _, err := fsm.MoveToEvaluating(ctx, "s1"); err == nil {
		t.Error("expected illegal transition error from COMPLETED, got nil")
	}
}

func TestFlowStateMachine_MarkCompleted_Idempotent(t *testing.T) {
	fsm := newTestStateMachine(t)
	ctx := context.Background()

	if _, err := fsm.EnsureInitialized(ctx, "s1", 2); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}
	st, err := fsm.MarkCompleted(ctx, "s1")
	if err != nil {
		t.Fatalf("MarkCompleted failed: %v", err)
	}
	if st.Status != models.FlowCompleted {
		t.Errorf("status = %s, want COMPLETED", st.Status)
	}
	// 重复标记幂等
	st, err = fsm.MarkCompleted(ctx, "s1")
	if err != nil {
		t.Fatalf("second MarkCompleted failed: %v", err)
	}
	if st.Status != models.FlowCompleted {
		t.Errorf("status = %s, want COMPLETED", st.Status)
	}
}

func TestFlowStateMachine_RestoreFlow(t *testing.T) {
	fsm := newTestStateMachine(t)
	ctx := context.Background()

	if _, err := fsm.EnsureInitialized(ctx, "s1", 3); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}
	current, _ := fsm.Current(ctx, "s1")
	snapshot := fsm.SnapshotFlow(current)

	if _, err := fsm.MoveToEvaluating(ctx, "s1"); err != nil {
		t.Fatalf("MoveToEvaluating failed: %v", err)
	}
	if err := fsm.RestoreFlow(ctx, "s1", snapshot); err != nil {
		t.Fatalf("RestoreFlow failed: %v", err)
	}

	restored, _ := fsm.Current(ctx, "s1")
	if restored.Status != models.FlowAsking {
		t.Errorf("restored status = %s, want ASKING", restored.Status)
	}
	if restored.Version != snapshot.Version {
		t.Errorf("restored version = %d, want %d", restored.Version, snapshot.Version)
	}
}
