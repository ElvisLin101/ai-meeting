package runtime

import (
	"ai-meeting/models"
	mongorepo "ai-meeting/repositories/mongo"
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// ============================================================
// SnapshotService 面试运行态快照服务
//
// 两条链路:
//   refreshSnapshot: 答题 commit 后把 Redis 运行态刷到 Mongo（CAS + 幂等短路）
//   ensureRuntime: Redis miss 后从 Mongo 快照+归档重建 Redis
// ============================================================

const (
	casMaxRetries      = 3
	casBaseBackoffMs   = 20
	rehydrateLockTTL   = 60 * time.Second
	rehydratePollCount = 4
	rehydratePollDelay = 80 * time.Millisecond
)

// SnapshotService 快照服务
type SnapshotService struct {
	rdb          *redis.Client
	flowCache    *FlowCache
	scoreCache   *ScoreCache
	turnLogCache *TurnLogCache
	questionCache *QuestionCache
}

// NewSnapshotService 创建快照服务
func NewSnapshotService(rdb *redis.Client, flowCache *FlowCache, scoreCache *ScoreCache, turnLogCache *TurnLogCache, questionCache *QuestionCache) *SnapshotService {
	return &SnapshotService{
		rdb:           rdb,
		flowCache:     flowCache,
		scoreCache:    scoreCache,
		turnLogCache:  turnLogCache,
		questionCache: questionCache,
	}
}

// RefreshAfterAnswerCommitted 答题 commit 后刷新快照到 Mongo
// flow: 当前 flow 状态, turn: 本轮 turn log, requestID: 幂等键
func (s *SnapshotService) RefreshAfterAnswerCommitted(ctx context.Context, sessionID, userID, requestID string, flow *models.InterviewFlowState, turn *models.InterviewTurnLog) {
	for attempt := 0; attempt < casMaxRetries; attempt++ {
		// 读当前热快照
		hot, _ := mongorepo.FindHotSnapshot(ctx, sessionID)

		// 幂等短路: lastMutationId == requestID 说明已落盘
		if hot != nil && hot.LastMutationID == requestID {
			return
		}

		// 归档本轮 turn
		var archiveWatermark int64
		var snapshotVersion int64 = 1
		if hot != nil {
			snapshotVersion = hot.SnapshotVersion + 1
		}
		if turn != nil {
			seq, err := mongorepo.ArchiveTurn(ctx, sessionID, requestID, *turn, snapshotVersion)
			if err != nil {
				logrus.Warnf("Failed to archive turn, session=%s, err=%v", sessionID, err)
			} else {
				archiveWatermark = seq
			}
		}

		// 构造热快照 patch
		newSnap := buildHotSnapshot(sessionID, userID, flow, hot, turn, requestID, archiveWatermark, snapshotVersion)

		// CAS 写入
		var ok bool
		var err error
		if hot != nil {
			ok, err = mongorepo.CompareAndSetHotSnapshot(ctx, sessionID, hot.SnapshotVersion, newSnap)
		} else {
			err = mongorepo.UpsertHotSnapshot(ctx, newSnap)
			ok = err == nil
		}
		if err != nil {
			logrus.Warnf("Hot snapshot CAS failed (attempt %d), session=%s, err=%v", attempt+1, sessionID, err)
			time.Sleep(time.Duration(casBaseBackoffMs*(attempt+1)) * time.Millisecond)
			continue
		}
		if ok {
			return
		}
		// CAS 失败(版本冲突), 退避重试
		time.Sleep(time.Duration(casBaseBackoffMs*(attempt+1)) * time.Millisecond)
	}
	logrus.Warnf("Hot snapshot CAS retries exhausted, session=%s", sessionID)
}

// RefreshColdSnapshot 出题后刷新冷快照到 Mongo
func (s *SnapshotService) RefreshColdSnapshot(ctx context.Context, sessionID, userID, interviewType, direction string, questions, suggestions map[string]string, resumeContext map[string]interface{}, resumeScore int) {
	snap := &models.InterviewRuntimeColdSnapshot{
		SessionID:       sessionID,
		UserID:          userID,
		MaterialVersion: 1,
		InterviewType:   interviewType,
		Direction:       direction,
		Questions:       questions,
		Suggestions:     suggestions,
		ResumeContext:   resumeContext,
		ResumeScore:     resumeScore,
	}
	// 已存在则只递增 materialVersion
	if existing, err := mongorepo.FindColdSnapshot(ctx, sessionID); err == nil && existing != nil {
		snap.MaterialVersion = existing.MaterialVersion + 1
	}
	if err := mongorepo.UpsertColdSnapshot(ctx, snap); err != nil {
		logrus.Warnf("Failed to upsert cold snapshot, session=%s, err=%v", sessionID, err)
	}
}

// EnsureRuntime Redis miss 后从 Mongo 重建运行态
// 返回重建的 flow, 或 nil 表示无法恢复
func (s *SnapshotService) EnsureRuntime(ctx context.Context, sessionID string) (*models.InterviewFlowState, error) {
	// 1. Redis 命中?
	flow, err := s.flowCache.GetFlow(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if flow != nil {
		return flow, nil // 命中, 零开销
	}

	// 2. 从热快照重建
	hot, err := mongorepo.FindHotSnapshot(ctx, sessionID)
	if err == nil && hot != nil {
		// 写回 Redis
		if err := s.flowCache.SaveFlow(ctx, sessionID, &hot.Flow); err != nil {
			logrus.Warnf("Failed to write flow back to Redis, session=%s, err=%v", sessionID, err)
		}
		// 恢复分数
		if hot.ScoreCount > 0 {
			avg := hot.ScoreSum / hot.ScoreCount
			s.rdb.Set(ctx, scoreKey(sessionID), fmt.Sprintf("%d", avg), cacheTTLHours*time.Hour)
			s.rdb.Set(ctx, scoreSumKey(sessionID), fmt.Sprintf("%d", hot.ScoreSum), cacheTTLHours*time.Hour)
			s.rdb.Set(ctx, scoreCountKey(sessionID), fmt.Sprintf("%d", hot.ScoreCount), cacheTTLHours*time.Hour)
		}
		// 恢复追问题
		if len(hot.FollowUpQuestions) > 0 {
			args := make([]interface{}, 0, len(hot.FollowUpQuestions)*2)
			for k, v := range hot.FollowUpQuestions {
				args = append(args, k, v)
			}
			s.rdb.HSet(ctx, followUpQuestionsKey(sessionID), args...)
		}
		logrus.Infof("Runtime rebuilt from hot snapshot, session=%s", sessionID)
		return &hot.Flow, nil
	}

	// 3. 从冷快照恢复材料
	cold, _ := mongorepo.FindColdSnapshot(ctx, sessionID)
	if cold != nil {
		s.questionCache.SaveQuestions(ctx, sessionID, cold.Questions)
		s.questionCache.SaveSuggestions(ctx, sessionID, cold.Suggestions)
		s.questionCache.SaveResumeContext(ctx, sessionID, cold.ResumeContext)
		s.questionCache.SaveResumeScore(ctx, sessionID, cold.ResumeScore)
		s.questionCache.SaveDirection(ctx, sessionID, cold.Direction)
	}

	// 4. 从 TurnArchive 恢复轮次
	archives, _ := mongorepo.FindTurnArchives(ctx, sessionID)
	if len(archives) > 0 {
		for _, arch := range archives {
			s.turnLogCache.AppendTurnIfAbsent(ctx, sessionID, &arch.TurnPayload)
		}
		logrus.Infof("Turns rebuilt from archive, session=%s, count=%d", sessionID, len(archives))
	}

	// 无法恢复 flow（快照和归档都没有 flow 信息）
	logrus.Warnf("Runtime rebuild failed: no hot snapshot for session=%s", sessionID)
	return nil, nil
}

// buildHotSnapshot 构造热快照
func buildHotSnapshot(sessionID, userID string, flow *models.InterviewFlowState, existing *models.InterviewRuntimeHotSnapshot, turn *models.InterviewTurnLog, requestID string, archiveWatermark, snapshotVersion int64) *models.InterviewRuntimeHotSnapshot {
	snap := &models.InterviewRuntimeHotSnapshot{
		SessionID:       sessionID,
		UserID:          userID,
		SnapshotVersion: snapshotVersion,
		SnapshotLevel:   "ACTIVE",
		LastMutationID:  requestID,
	}

	if flow != nil {
		snap.Flow = *flow
		snap.LastCommittedQuestionNumber = flow.CurrentQuestionNumber
	}

	// 保留已有追问题
	if existing != nil {
		snap.FollowUpQuestions = existing.FollowUpQuestions
		snap.ScoreSum = existing.ScoreSum
		snap.ScoreCount = existing.ScoreCount
		snap.RecentTurns = existing.RecentTurns
		snap.RecentTurnCount = existing.RecentTurnCount
		snap.LastTurnSeq = existing.LastTurnSeq
		snap.ArchiveWatermark = existing.ArchiveWatermark
	}

	// 追加本轮 turn 到窗口
	if turn != nil {
		snap.RecentTurns = append(snap.RecentTurns, *turn)
		// 限制窗口大小
		if len(snap.RecentTurns) > models.HotSnapshotRecentTurnLimit {
			snap.RecentTurns = snap.RecentTurns[len(snap.RecentTurns)-models.HotSnapshotRecentTurnLimit:]
		}
		snap.RecentTurnCount = len(snap.RecentTurns)
		if archiveWatermark > 0 {
			snap.ArchiveWatermark = archiveWatermark
			snap.LastTurnSeq = archiveWatermark
		}
		snap.LastAppliedRequestID = requestID
	}

	return snap
}
