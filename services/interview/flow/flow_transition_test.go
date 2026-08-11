package flow

import (
	"testing"

	"ai-meeting/models"
)

// ============================================================
// 状态机转移合法性测试 + 快照深拷贝测试
// ============================================================

func TestAssertLegalTransition(t *testing.T) {
	legal := [][2]models.InterviewFlowStatus{
		{models.FlowInit, models.FlowAsking},
		{models.FlowInit, models.FlowCompleted},
		{models.FlowAsking, models.FlowEvaluating},
		{models.FlowAsking, models.FlowFollowUp},
		{models.FlowAsking, models.FlowCompleted},
		{models.FlowEvaluating, models.FlowAsking},
		{models.FlowEvaluating, models.FlowFollowUp},
		{models.FlowEvaluating, models.FlowCompleted},
		{models.FlowFollowUp, models.FlowEvaluating},
		{models.FlowFollowUp, models.FlowAsking},
		{models.FlowFollowUp, models.FlowCompleted},
	}
	for _, tr := range legal {
		if err := assertLegalTransition(tr[0], tr[1]); err != nil {
			t.Errorf("合法转移 %s→%s 应通过, got %v", tr[0], tr[1], err)
		}
	}

	illegal := [][2]models.InterviewFlowStatus{
		{models.FlowInit, models.FlowEvaluating},
		{models.FlowInit, models.FlowFollowUp},
		{models.FlowCompleted, models.FlowAsking},
		{models.FlowCompleted, models.FlowEvaluating},
		{models.FlowCompleted, models.FlowFollowUp},
		{models.FlowAsking, models.FlowInit},
		{models.FlowEvaluating, models.FlowInit},
		{models.FlowFollowUp, models.FlowInit},
	}
	for _, tr := range illegal {
		if err := assertLegalTransition(tr[0], tr[1]); err == nil {
			t.Errorf("非法转移 %s→%s 应报错", tr[0], tr[1])
		}
	}
}

func TestAssertLegalTransition_UnknownSource(t *testing.T) {
	if err := assertLegalTransition(models.InterviewFlowStatus("NOPE"), models.FlowAsking); err == nil {
		t.Error("未知源状态应报错")
	}
}

func TestSnapshotFlow_DeepCopy(t *testing.T) {
	m := &FlowStateMachine{}
	state := &models.InterviewFlowState{
		Status:                models.FlowAsking,
		CurrentIndex:          2,
		CurrentQuestionNumber: "3",
		TotalQuestions:        5,
		FollowUpCount:         1,
		MaxFollowUp:           2,
		Version:               3,
	}

	cp := m.SnapshotFlow(state)
	if cp == state {
		t.Fatal("SnapshotFlow 应返回深拷贝, 而非同一指针")
	}
	if cp.Status != state.Status || cp.CurrentIndex != 2 || cp.Version != 3 {
		t.Errorf("拷贝内容不一致: %+v", cp)
	}

	// 修改副本不应影响原对象
	cp.CurrentIndex = 99
	cp.Status = models.FlowCompleted
	if state.CurrentIndex != 2 || state.Status != models.FlowAsking {
		t.Error("副本修改污染了原对象")
	}
}

func TestSnapshotFlow_Nil(t *testing.T) {
	m := &FlowStateMachine{}
	if m.SnapshotFlow(nil) != nil {
		t.Error("nil 输入应返回 nil")
	}
}
