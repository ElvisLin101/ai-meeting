package mysql

import (
	"ai-meeting/models"
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ============================================================
// 面试运行态 MySQL 仓储
// 热快照 CAS / 冷快照 upsert / 轮次归档幂等
// ============================================================

// FindHotSnapshot 查询热快照（不存在返回 nil, nil）
func FindHotSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeHotSnapshot, error) {
	var row models.InterviewHotSnapshotRow
	err := DB.WithContext(ctx).Where("session_id = ?", sessionID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return hotRowToModel(&row), nil
}

// UpsertHotSnapshot 覆盖写入热快照（无 CAS）
func UpsertHotSnapshot(ctx context.Context, snap *models.InterviewRuntimeHotSnapshot) error {
	row := hotModelToRow(snap)
	row.UpdatedAt = time.Now()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = row.UpdatedAt
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "session_status", "snapshot_version", "snapshot_level",
			"flow_json", "score_sum", "score_count", "follow_up_questions_json",
			"recent_turns_json", "recent_turn_count", "archive_watermark",
			"last_turn_seq", "last_applied_request_id", "last_mutation_id",
			"last_committed_question_number", "updated_at",
		}),
	}).Create(row).Error
}

// CompareAndSetHotSnapshot CAS 更新热快照
func CompareAndSetHotSnapshot(ctx context.Context, sessionID string, expectedVersion int64, snap *models.InterviewRuntimeHotSnapshot) (bool, error) {
	row := hotModelToRow(snap)
	row.UpdatedAt = time.Now()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = row.UpdatedAt
	}
	updates := map[string]interface{}{
		"session_status":                 row.SessionStatus,
		"snapshot_version":               row.SnapshotVersion,
		"snapshot_level":                 row.SnapshotLevel,
		"flow_json":                      row.FlowJSON,
		"score_sum":                      row.ScoreSum,
		"score_count":                    row.ScoreCount,
		"follow_up_questions_json":       row.FollowUpQuestionsJSON,
		"recent_turns_json":              row.RecentTurnsJSON,
		"recent_turn_count":              row.RecentTurnCount,
		"archive_watermark":              row.ArchiveWatermark,
		"last_turn_seq":                  row.LastTurnSeq,
		"last_applied_request_id":        row.LastAppliedRequestID,
		"last_mutation_id":               row.LastMutationID,
		"last_committed_question_number": row.LastCommittedQuestionNumber,
		"updated_at":                     row.UpdatedAt,
	}
	res := DB.WithContext(ctx).Model(&models.InterviewHotSnapshotRow{}).
		Where("session_id = ? AND snapshot_version = ?", sessionID, expectedVersion).
		Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected > 0 {
		return true, nil
	}

	// 记录不存在 → 初始插入；并发插入冲突则返回失败
	var count int64
	if err := DB.WithContext(ctx).Model(&models.InterviewHotSnapshotRow{}).
		Where("session_id = ?", sessionID).Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// FindColdSnapshot 查询冷快照（不存在返回 nil, nil）
func FindColdSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeColdSnapshot, error) {
	var row models.InterviewColdSnapshotRow
	err := DB.WithContext(ctx).Where("session_id = ?", sessionID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return coldRowToModel(&row), nil
}

// UpsertColdSnapshot 写入冷快照
func UpsertColdSnapshot(ctx context.Context, snap *models.InterviewRuntimeColdSnapshot) error {
	row := coldModelToRow(snap)
	row.UpdatedAt = time.Now()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = row.UpdatedAt
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "material_version", "interview_type", "direction",
			"questions_json", "suggestions_json", "resume_context_json",
			"resume_score", "updated_at",
		}),
	}).Create(row).Error
}

// ArchiveTurn 归档一轮（按 requestId 幂等），返回 seq
func ArchiveTurn(ctx context.Context, sessionID, requestID string, turn models.InterviewTurnLog, snapshotVersion int64) (int64, error) {
	var existing models.InterviewTurnArchiveRow
	err := DB.WithContext(ctx).Where("session_id = ? AND request_id = ?", sessionID, requestID).
		First(&existing).Error
	if err == nil {
		return existing.Seq, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}

	seq, err := NextSessionSequence(DB.WithContext(ctx), "turn:"+sessionID)
	if err != nil {
		return 0, err
	}
	payload, _ := json.Marshal(turn)
	row := &models.InterviewTurnArchiveRow{
		SessionID:       sessionID,
		RequestID:       requestID,
		Seq:             seq,
		SnapshotVersion: snapshotVersion,
		TurnPayloadJSON: string(payload),
		CreatedAt:       time.Now(),
	}
	if err := DB.WithContext(ctx).Create(row).Error; err != nil {
		// 并发重复插入 → 回读已有记录
		if err2 := DB.WithContext(ctx).Where("session_id = ? AND request_id = ?", sessionID, requestID).
			First(&existing).Error; err2 == nil {
			return existing.Seq, nil
		}
		return 0, err
	}
	return seq, nil
}

// FindTurnArchives 按 seq 正序查询轮次归档
func FindTurnArchives(ctx context.Context, sessionID string) ([]models.InterviewSessionTurnArchive, error) {
	var rows []models.InterviewTurnArchiveRow
	if err := DB.WithContext(ctx).Where("session_id = ?", sessionID).
		Order("seq ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.InterviewSessionTurnArchive, 0, len(rows))
	for _, row := range rows {
		var turn models.InterviewTurnLog
		_ = json.Unmarshal([]byte(row.TurnPayloadJSON), &turn)
		out = append(out, models.InterviewSessionTurnArchive{
			ID: row.SessionID + ":" + row.RequestID, SessionID: row.SessionID,
			RequestID: row.RequestID, Seq: row.Seq, SnapshotVersion: row.SnapshotVersion,
			TurnPayload: turn, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

// ============================================================
// 模型 <-> 行 转换
// ============================================================

func hotModelToRow(snap *models.InterviewRuntimeHotSnapshot) *models.InterviewHotSnapshotRow {
	flow, _ := json.Marshal(snap.Flow)
	followUps, _ := json.Marshal(snap.FollowUpQuestions)
	turns, _ := json.Marshal(snap.RecentTurns)
	return &models.InterviewHotSnapshotRow{
		SessionID: snap.SessionID, UserID: snap.UserID,
		SessionStatus: snap.SessionStatus, SnapshotVersion: snap.SnapshotVersion,
		SnapshotLevel: snap.SnapshotLevel, FlowJSON: string(flow),
		ScoreSum: snap.ScoreSum, ScoreCount: snap.ScoreCount,
		FollowUpQuestionsJSON: string(followUps), RecentTurnsJSON: string(turns),
		RecentTurnCount: snap.RecentTurnCount, ArchiveWatermark: snap.ArchiveWatermark,
		LastTurnSeq: snap.LastTurnSeq, LastAppliedRequestID: snap.LastAppliedRequestID,
		LastMutationID:              snap.LastMutationID,
		LastCommittedQuestionNumber: snap.LastCommittedQuestionNumber,
		CreatedAt:                   snap.CreatedAt, UpdatedAt: snap.UpdatedAt,
	}
}

func hotRowToModel(row *models.InterviewHotSnapshotRow) *models.InterviewRuntimeHotSnapshot {
	snap := &models.InterviewRuntimeHotSnapshot{
		SessionID: row.SessionID, UserID: row.UserID,
		SessionStatus: row.SessionStatus, SnapshotVersion: row.SnapshotVersion,
		SnapshotLevel: row.SnapshotLevel, ScoreSum: row.ScoreSum, ScoreCount: row.ScoreCount,
		RecentTurnCount: row.RecentTurnCount, ArchiveWatermark: row.ArchiveWatermark,
		LastTurnSeq: row.LastTurnSeq, LastAppliedRequestID: row.LastAppliedRequestID,
		LastMutationID:              row.LastMutationID,
		LastCommittedQuestionNumber: row.LastCommittedQuestionNumber,
		CreatedAt:                   row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	_ = json.Unmarshal([]byte(row.FlowJSON), &snap.Flow)
	_ = json.Unmarshal([]byte(row.FollowUpQuestionsJSON), &snap.FollowUpQuestions)
	_ = json.Unmarshal([]byte(row.RecentTurnsJSON), &snap.RecentTurns)
	return snap
}

func coldModelToRow(snap *models.InterviewRuntimeColdSnapshot) *models.InterviewColdSnapshotRow {
	questions, _ := json.Marshal(snap.Questions)
	suggestions, _ := json.Marshal(snap.Suggestions)
	resumeCtx, _ := json.Marshal(snap.ResumeContext)
	return &models.InterviewColdSnapshotRow{
		SessionID: snap.SessionID, UserID: snap.UserID,
		MaterialVersion: snap.MaterialVersion, InterviewType: snap.InterviewType,
		Direction: snap.Direction, QuestionsJSON: string(questions),
		SuggestionsJSON: string(suggestions), ResumeContextJSON: string(resumeCtx),
		ResumeScore: snap.ResumeScore, CreatedAt: snap.CreatedAt, UpdatedAt: snap.UpdatedAt,
	}
}

func coldRowToModel(row *models.InterviewColdSnapshotRow) *models.InterviewRuntimeColdSnapshot {
	snap := &models.InterviewRuntimeColdSnapshot{
		SessionID: row.SessionID, UserID: row.UserID,
		MaterialVersion: row.MaterialVersion, InterviewType: row.InterviewType,
		Direction: row.Direction, ResumeScore: row.ResumeScore,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	_ = json.Unmarshal([]byte(row.QuestionsJSON), &snap.Questions)
	_ = json.Unmarshal([]byte(row.SuggestionsJSON), &snap.Suggestions)
	_ = json.Unmarshal([]byte(row.ResumeContextJSON), &snap.ResumeContext)
	return snap
}
