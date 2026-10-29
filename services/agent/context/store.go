package agentctx

import (
	"ai-meeting/models"
	"ai-meeting/repositories"
	"context"
	"encoding/json"

	"github.com/sirupsen/logrus"
)

// ============================================================
// 上下文数据读写: Redis 优先, miss 从 MySQL 回填
// ============================================================

// LoadRecentMessages 读取近期消息（Redis 优先, miss 从 MySQL 重建）
func LoadRecentMessages(ctx context.Context, sessionID, userID string) ([]models.Message, error) {
	return LoadRecentMessagesAfter(ctx, sessionID, userID, 0)
}

// LoadRecentMessagesAfter 读取 sequence > afterSeq 的短期消息。
// afterSeq 是中期摘要的 CoveredThroughSeq：摘要与短期消息以它为界，不能重叠。
func LoadRecentMessagesAfter(ctx context.Context, sessionID, userID string, afterSeq int64) ([]models.Message, error) {
	const limit = 500

	recent, err := repositories.GetRecentMessages(ctx, sessionID)
	if err != nil {
		logrus.Warnf("[agent-ctx] read redis recent failed, fallback mysql: %v", err)
	} else if len(recent) > 0 {
		filtered := make([]models.Message, 0, len(recent))
		for _, m := range recent {
			if m.Sequence > afterSeq {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) > 0 {
			return filtered, nil
		}
	}

	messages, err := currentRepository().ListRecentMessagesAfter(sessionID, userID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	if len(messages) > 0 {
		// 重建 Redis 时只放短期消息，保证缓存和 afterSeq 边界一致。
		if err := repositories.RebuildRecentMessages(ctx, sessionID, messages); err != nil {
			logrus.Warnf("[agent-ctx] rebuild redis recent failed: %v", err)
		}
	}
	return messages, nil
}

// LoadSummary 读取压缩摘要与边界（Redis 优先, miss 从 MySQL 回填）
func LoadSummary(ctx context.Context, sessionID string) (string, int64) {
	if summary, index, ok := repositories.GetCompressedSummary(ctx, sessionID); ok {
		return summary, index
	}
	record, err := currentRepository().FindCompressedContext(sessionID)
	if err != nil {
		logrus.Warnf("[agent-ctx] read compressed context failed: %v", err)
		return "", 0
	}
	if record == nil || record.CompressedContent == "" || record.Index <= 0 {
		return "", 0
	}
	if err := repositories.SetCompressedSummary(ctx, sessionID, record.CompressedContent, record.Index); err != nil {
		logrus.Warnf("[agent-ctx] backfill summary to redis failed: %v", err)
	}
	return record.CompressedContent, record.Index
}

// SaveSummary 写入压缩摘要（MySQL 先写, Redis 后写）
func SaveSummary(ctx context.Context, sessionID, userID, summary string, index int64, totalLength, count int) error {
	record := &models.CompressedContextRecord{
		SessionID:         sessionID,
		UserID:            userID,
		MemoryScope:       "agent",
		CompressedContent: summary,
		Index:             index,
		TotalLength:       totalLength,
		MessageCount:      count,
	}
	if err := currentRepository().UpsertCompressedContext(record); err != nil {
		return err
	}
	if err := repositories.SetCompressedSummary(ctx, sessionID, summary, index); err != nil {
		logrus.Warnf("[agent-ctx] write summary to redis failed: %v", err)
	}
	return nil
}

// LoadProfile 读取用户画像（Redis 优先, miss 从 MySQL 回填）
func LoadProfile(ctx context.Context, userID string) *UserProfileData {
	if raw, ok := repositories.GetUserProfile(ctx, userID); ok {
		var profile UserProfileData
		if err := json.Unmarshal([]byte(raw), &profile); err == nil {
			NormalizeProfile(&profile)
			return &profile
		}
	}
	record, err := currentRepository().FindUserProfile(userID)
	if err != nil || record == nil || record.ProfileJSON == "" {
		return nil
	}
	var profile UserProfileData
	if err := json.Unmarshal([]byte(record.ProfileJSON), &profile); err != nil {
		logrus.Warnf("[agent-ctx] unmarshal user profile failed: %v", err)
		return nil
	}
	NormalizeProfile(&profile)
	if profile.LastConsolidatedSeq == 0 {
		profile.LastConsolidatedSeq = record.LastConsolidatedSeq
	}
	if err := repositories.SetUserProfile(ctx, userID, record.ProfileJSON); err != nil {
		logrus.Warnf("[agent-ctx] backfill profile to redis failed: %v", err)
	}
	return &profile
}

// SaveProfile 写入用户画像（MySQL 先写, Redis 后写）
func SaveProfile(ctx context.Context, userID string, profile *UserProfileData) error {
	NormalizeProfile(profile)
	profile.UpdatedAt = nowString()
	if profile.Version <= 0 {
		profile.Version = 1
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	record := &models.UserProfile{
		UserID:              userID,
		ProfileJSON:         string(raw),
		SchemaVersion:       1,
		Version:             profile.Version,
		LastConsolidatedSeq: profile.LastConsolidatedSeq,
	}
	if err := currentRepository().UpsertUserProfile(record); err != nil {
		return err
	}
	if err := repositories.SetUserProfile(ctx, userID, string(raw)); err != nil {
		logrus.Warnf("[agent-ctx] write profile to redis failed: %v", err)
	}
	return nil
}

// LoadIdentity 读取 Agent 自我定位（Redis -> MySQL -> 默认值）
func LoadIdentity(ctx context.Context, agentID uint) *AgentIdentityData {
	if raw, ok := repositories.GetAgentIdentity(ctx, agentID); ok {
		var identity AgentIdentityData
		if err := json.Unmarshal([]byte(raw), &identity); err == nil {
			return &identity
		}
	}
	record, err := currentRepository().FindAgentIdentity(agentID)
	if err != nil || record == nil || record.IdentityJSON == "" {
		return DefaultIdentity()
	}
	var identity AgentIdentityData
	if err := json.Unmarshal([]byte(record.IdentityJSON), &identity); err != nil {
		return DefaultIdentity()
	}
	if err := repositories.SetAgentIdentity(ctx, agentID, record.IdentityJSON); err != nil {
		logrus.Warnf("[agent-ctx] backfill identity to redis failed: %v", err)
	}
	return &identity
}

// SaveIdentity 写入 Agent 自我定位
func SaveIdentity(ctx context.Context, agentID uint, identity *AgentIdentityData) error {
	if identity.Version <= 0 {
		identity.Version = 1
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	record := &models.AgentIdentity{
		AgentID:      agentID,
		IdentityJSON: string(raw),
		Version:      identity.Version,
	}
	if err := currentRepository().UpsertAgentIdentity(record); err != nil {
		return err
	}
	if err := repositories.SetAgentIdentity(ctx, agentID, string(raw)); err != nil {
		logrus.Warnf("[agent-ctx] write identity to redis failed: %v", err)
	}
	return nil
}
