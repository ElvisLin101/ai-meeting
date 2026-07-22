package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ============================================================
// 面试运行态 Mongo 仓储
// 热快照: CAS 乐观锁更新
// 冷快照: 无 CAS, upsert 覆盖
// 轮次归档: 不可变, 按 requestId 幂等, seq 单调
// ============================================================

const (
	hotSnapshotCollection   = "interview_session_runtime_hot_snapshot"
	coldSnapshotCollection  = "interview_session_runtime_cold_snapshot"
	turnArchiveCollection   = "interview_session_turn_archive"
)

// ============================================================
// 热快照
// ============================================================

// FindHotSnapshot 按 sessionID 查热快照
func FindHotSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeHotSnapshot, error) {
	collection, err := GetCollection(hotSnapshotCollection)
	if err != nil {
		return nil, err
	}
	var snap models.InterviewRuntimeHotSnapshot
	err = collection.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&snap)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// UpsertHotSnapshot upsert 热快照（无 CAS, 用于初始化和回滚）
func UpsertHotSnapshot(ctx context.Context, snap *models.InterviewRuntimeHotSnapshot) error {
	collection, err := GetCollection(hotSnapshotCollection)
	if err != nil {
		return err
	}
	now := time.Now()
	snap.UpdatedAt = now
	update := bson.M{
		"$set": bson.M{
			"user_id":                     snap.UserID,
			"session_status":              snap.SessionStatus,
			"snapshot_version":            snap.SnapshotVersion,
			"snapshot_level":              snap.SnapshotLevel,
			"flow":                        snap.Flow,
			"score_sum":                   snap.ScoreSum,
			"score_count":                 snap.ScoreCount,
			"follow_up_questions":         snap.FollowUpQuestions,
			"recent_turns":                snap.RecentTurns,
			"recent_turn_count":           snap.RecentTurnCount,
			"archive_watermark":           snap.ArchiveWatermark,
			"last_turn_seq":               snap.LastTurnSeq,
			"last_applied_request_id":     snap.LastAppliedRequestID,
			"last_mutation_id":            snap.LastMutationID,
			"last_committed_question_number": snap.LastCommittedQuestionNumber,
			"updated_at":                  now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": snap.SessionID}, update, opts)
	return err
}

// CompareAndSetHotSnapshot CAS 更新热快照
// filter 带 snapshot_version 前置条件, modifiedCount>0 才成功
func CompareAndSetHotSnapshot(ctx context.Context, sessionID string, expectedVersion int64, snap *models.InterviewRuntimeHotSnapshot) (bool, error) {
	collection, err := GetCollection(hotSnapshotCollection)
	if err != nil {
		return false, err
	}
	now := time.Now()
	snap.UpdatedAt = now
	filter := bson.M{
		"_id":               sessionID,
		"snapshot_version":  expectedVersion,
	}
	update := bson.M{
		"$set": bson.M{
			"session_status":              snap.SessionStatus,
			"snapshot_version":            snap.SnapshotVersion,
			"snapshot_level":              snap.SnapshotLevel,
			"flow":                        snap.Flow,
			"score_sum":                   snap.ScoreSum,
			"score_count":                 snap.ScoreCount,
			"follow_up_questions":         snap.FollowUpQuestions,
			"recent_turns":                snap.RecentTurns,
			"recent_turn_count":           snap.RecentTurnCount,
			"archive_watermark":           snap.ArchiveWatermark,
			"last_turn_seq":               snap.LastTurnSeq,
			"last_applied_request_id":     snap.LastAppliedRequestID,
			"last_mutation_id":            snap.LastMutationID,
			"last_committed_question_number": snap.LastCommittedQuestionNumber,
			"updated_at":                  now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
			"user_id":    snap.UserID,
		},
	}
	opts := options.Update().SetUpsert(true)
	result, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, err
	}
	return result.ModifiedCount > 0 || result.UpsertedCount > 0, nil
}

// ============================================================
// 冷快照
// ============================================================

// FindColdSnapshot 按 sessionID 查冷快照
func FindColdSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeColdSnapshot, error) {
	collection, err := GetCollection(coldSnapshotCollection)
	if err != nil {
		return nil, err
	}
	var snap models.InterviewRuntimeColdSnapshot
	err = collection.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&snap)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// UpsertColdSnapshot upsert 冷快照（无 CAS）
func UpsertColdSnapshot(ctx context.Context, snap *models.InterviewRuntimeColdSnapshot) error {
	collection, err := GetCollection(coldSnapshotCollection)
	if err != nil {
		return err
	}
	now := time.Now()
	snap.UpdatedAt = now
	update := bson.M{
		"$set": bson.M{
			"user_id":          snap.UserID,
			"material_version": snap.MaterialVersion,
			"interview_type":   snap.InterviewType,
			"direction":        snap.Direction,
			"questions":        snap.Questions,
			"suggestions":      snap.Suggestions,
			"resume_context":   snap.ResumeContext,
			"resume_score":     snap.ResumeScore,
			"updated_at":       now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": snap.SessionID}, update, opts)
	return err
}

// ============================================================
// 轮次归档
// ============================================================

// ArchiveTurn 归档一轮, 按 requestId 幂等, 返回 seq
func ArchiveTurn(ctx context.Context, sessionID, requestID string, turn models.InterviewTurnLog, snapshotVersion int64) (int64, error) {
	collection, err := GetCollection(turnArchiveCollection)
	if err != nil {
		return 0, err
	}

	// 幂等: 先按 requestId 查重
	var existing models.InterviewSessionTurnArchive
	err = collection.FindOne(ctx, bson.M{"session_id": sessionID, "request_id": requestID}).Decode(&existing)
	if err == nil {
		return existing.Seq, nil // 已存在, 返回原 seq
	}

	// 取 max seq + 1
	maxSeq, err := nextTurnSeq(ctx, collection, sessionID)
	if err != nil {
		return 0, err
	}

	archive := models.InterviewSessionTurnArchive{
		ID:              sessionID + ":" + requestID,
		SessionID:       sessionID,
		RequestID:       requestID,
		Seq:             maxSeq,
		SnapshotVersion: snapshotVersion,
		TurnPayload:     turn,
		CreatedAt:       time.Now(),
	}
	_, err = collection.InsertOne(ctx, archive)
	if err != nil {
		return 0, err
	}
	return maxSeq, nil
}

// FindTurnArchives 按 sessionID 查全部归档（按 seq 正序）
func FindTurnArchives(ctx context.Context, sessionID string) ([]models.InterviewSessionTurnArchive, error) {
	collection, err := GetCollection(turnArchiveCollection)
	if err != nil {
		return nil, err
	}
	opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}})
	cursor, err := collection.Find(ctx, bson.M{"session_id": sessionID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var archives []models.InterviewSessionTurnArchive
	if err := cursor.All(ctx, &archives); err != nil {
		return nil, err
	}
	return archives, nil
}

// nextTurnSeq 取当前 session 的 max seq + 1
func nextTurnSeq(ctx context.Context, collection *drivermongo.Collection, sessionID string) (int64, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "seq", Value: -1}})
	var latest models.InterviewSessionTurnArchive
	err := collection.FindOne(ctx, bson.M{"session_id": sessionID}, opts).Decode(&latest)
	if err == drivermongo.ErrNoDocuments {
		return 1, nil // 第一条
	}
	if err != nil {
		return 0, err
	}
	return latest.Seq + 1, nil
}
