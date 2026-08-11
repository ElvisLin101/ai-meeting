package runtime

import (
	"fmt"
	"testing"

	"ai-meeting/models"
)

// ============================================================
// 热快照构造测试(buildHotSnapshot)
// ============================================================

func TestBuildHotSnapshot_Basic(t *testing.T) {
	flow := &models.InterviewFlowState{
		Status:                models.FlowAsking,
		CurrentIndex:          1,
		CurrentQuestionNumber: "2",
		Version:               3,
	}
	snap := buildHotSnapshot("s1", "u1", flow, nil, nil, "req-1", 0, 1)

	if snap.SessionID != "s1" || snap.UserID != "u1" {
		t.Errorf("session/user = %s/%s", snap.SessionID, snap.UserID)
	}
	if snap.SnapshotVersion != 1 {
		t.Errorf("SnapshotVersion = %d, want 1", snap.SnapshotVersion)
	}
	if snap.SnapshotLevel != "ACTIVE" {
		t.Errorf("SnapshotLevel = %q, want ACTIVE", snap.SnapshotLevel)
	}
	if snap.LastMutationID != "req-1" {
		t.Errorf("LastMutationID = %q, want req-1", snap.LastMutationID)
	}
	if snap.Flow.Status != models.FlowAsking || snap.Flow.Version != 3 {
		t.Errorf("Flow 未拷贝: %+v", snap.Flow)
	}
	if snap.LastCommittedQuestionNumber != "2" {
		t.Errorf("LastCommittedQuestionNumber = %q, want 2", snap.LastCommittedQuestionNumber)
	}
	if snap.RecentTurns != nil || snap.RecentTurnCount != 0 {
		t.Errorf("无 turn 时 RecentTurns 应为空: %+v", snap.RecentTurns)
	}
}

func TestBuildHotSnapshot_PreserveExisting(t *testing.T) {
	existing := &models.InterviewRuntimeHotSnapshot{
		ScoreSum:          100,
		ScoreCount:        2,
		FollowUpQuestions: map[string]string{"1-F1": "追问题"},
		RecentTurns:       []models.InterviewTurnLog{{QuestionNumber: "1"}},
		RecentTurnCount:   1,
		ArchiveWatermark:  5,
		LastTurnSeq:       5,
	}
	snap := buildHotSnapshot("s1", "u1", nil, existing, nil, "req-2", 0, 2)

	if snap.ScoreSum != 100 || snap.ScoreCount != 2 {
		t.Errorf("分数未保留: sum=%d count=%d", snap.ScoreSum, snap.ScoreCount)
	}
	if snap.FollowUpQuestions["1-F1"] != "追问题" {
		t.Errorf("追问题未保留: %v", snap.FollowUpQuestions)
	}
	if len(snap.RecentTurns) != 1 || snap.RecentTurnCount != 1 {
		t.Errorf("RecentTurns 未保留: %v", snap.RecentTurns)
	}
	if snap.ArchiveWatermark != 5 || snap.LastTurnSeq != 5 {
		t.Errorf("水位线未保留: wm=%d seq=%d", snap.ArchiveWatermark, snap.LastTurnSeq)
	}
}

func TestBuildHotSnapshot_WithTurn(t *testing.T) {
	turn := &models.InterviewTurnLog{QuestionNumber: "1", Score: 88}
	snap := buildHotSnapshot("s1", "u1", nil, nil, turn, "req-3", 7, 1)

	if len(snap.RecentTurns) != 1 || snap.RecentTurns[0].Score != 88 {
		t.Errorf("本轮 turn 未追加: %+v", snap.RecentTurns)
	}
	if snap.RecentTurnCount != 1 {
		t.Errorf("RecentTurnCount = %d, want 1", snap.RecentTurnCount)
	}
	if snap.ArchiveWatermark != 7 || snap.LastTurnSeq != 7 {
		t.Errorf("归档水位未更新: wm=%d seq=%d", snap.ArchiveWatermark, snap.LastTurnSeq)
	}
	if snap.LastAppliedRequestID != "req-3" {
		t.Errorf("LastAppliedRequestID = %q, want req-3", snap.LastAppliedRequestID)
	}
}

func TestBuildHotSnapshot_TurnWindowTrim(t *testing.T) {
	// 已有 20 轮(达到窗口上限), 追加 1 轮 → 裁剪保留最近 20 轮
	existing := &models.InterviewRuntimeHotSnapshot{}
	for i := 0; i < models.HotSnapshotRecentTurnLimit; i++ {
		existing.RecentTurns = append(existing.RecentTurns, models.InterviewTurnLog{
			QuestionNumber: fmt.Sprintf("%d", i),
		})
	}
	existing.RecentTurnCount = models.HotSnapshotRecentTurnLimit

	turn := &models.InterviewTurnLog{QuestionNumber: "new-turn"}
	snap := buildHotSnapshot("s1", "u1", nil, existing, turn, "req-4", 9, 1)

	if len(snap.RecentTurns) != models.HotSnapshotRecentTurnLimit {
		t.Fatalf("窗口未裁剪: len = %d, want %d", len(snap.RecentTurns), models.HotSnapshotRecentTurnLimit)
	}
	// 裁剪后保留最近 20 轮: 最早的 "0" 被丢弃, 最新的 "new-turn" 保留
	if snap.RecentTurns[0].QuestionNumber != "1" {
		t.Errorf("最早的轮次应被丢弃, 第一条 = %q", snap.RecentTurns[0].QuestionNumber)
	}
	last := snap.RecentTurns[len(snap.RecentTurns)-1]
	if last.QuestionNumber != "new-turn" {
		t.Errorf("最后一条应为新 turn, got %q", last.QuestionNumber)
	}
	if snap.RecentTurnCount != models.HotSnapshotRecentTurnLimit {
		t.Errorf("RecentTurnCount = %d", snap.RecentTurnCount)
	}
}
