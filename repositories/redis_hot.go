package repositories

import (
	"ai-meeting/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// ============================================================
// Agent 热数据（Redis）
//
// 定位: 纯缓存, 可丢失。MySQL 是真相源, 读 miss 时由上层从 MySQL 回填。
// 所有 key 带 7 天 TTL, 避免热数据无限膨胀。
// ============================================================

const (
	hotDataTTL = 7 * 24 * time.Hour
)

// RecentMessagesKey 会话近期消息 List key
func RecentMessagesKey(sessionID string) string { return "agent:session:" + sessionID + ":recent" }

// SummaryKey 远期压缩摘要 key
func SummaryKey(sessionID string) string { return "agent:session:" + sessionID + ":summary" }

// SummaryIndexKey 压缩边界 key
func SummaryIndexKey(sessionID string) string { return "agent:session:" + sessionID + ":summary_idx" }

// UserProfileKey 用户画像 key
func UserProfileKey(userID string) string { return "user:" + userID + ":profile" }

// AgentIdentityKey Agent 自我定位 key
func AgentIdentityKey(agentID uint) string { return fmt.Sprintf("agent:%d:identity", agentID) }

// PushRecentMessage 追加一条近期消息。
// 这里不做 LTRIM：未经摘要压缩的消息不能直接丢弃。
// 短期列表的裁剪由压缩流程在推进 CoveredThroughSeq 后重建。
func PushRecentMessage(ctx context.Context, sessionID string, msg *models.Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	key := RecentMessagesKey(sessionID)
	pipe := RedisClient.Pipeline()
	pipe.LPush(ctx, key, payload)
	pipe.Expire(ctx, key, hotDataTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// GetRecentMessages 读取近期消息（按 sequence 正序）
func GetRecentMessages(ctx context.Context, sessionID string) ([]models.Message, error) {
	key := RecentMessagesKey(sessionID)
	vals, err := RedisClient.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	messages := make([]models.Message, 0, len(vals))
	// List 是 LPUSH 写入, 取出来是倒序, 反转成正序
	for i := len(vals) - 1; i >= 0; i-- {
		var msg models.Message
		if err := json.Unmarshal([]byte(vals[i]), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// RebuildRecentMessages 用 MySQL 消息重建 Redis 近期列表（回填 / 压缩后裁剪）。
func RebuildRecentMessages(ctx context.Context, sessionID string, messages []models.Message) error {
	key := RecentMessagesKey(sessionID)
	pipe := RedisClient.Pipeline()
	pipe.Del(ctx, key)
	if len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			payload, err := json.Marshal(messages[i])
			if err != nil {
				continue
			}
			pipe.LPush(ctx, key, payload)
		}
		pipe.Expire(ctx, key, hotDataTTL)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// DeleteRecentMessages 删除会话近期消息
func DeleteRecentMessages(ctx context.Context, sessionID string) error {
	return RedisClient.Del(ctx, RecentMessagesKey(sessionID)).Err()
}

// SetCompressedSummary 写入压缩摘要与边界
func SetCompressedSummary(ctx context.Context, sessionID, summary string, index int64) error {
	pipe := RedisClient.Pipeline()
	pipe.Set(ctx, SummaryKey(sessionID), summary, hotDataTTL)
	pipe.Set(ctx, SummaryIndexKey(sessionID), index, hotDataTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// GetCompressedSummary 读取压缩摘要与边界；不存在返回 ("", 0, false)
func GetCompressedSummary(ctx context.Context, sessionID string) (string, int64, bool) {
	summary, err := RedisClient.Get(ctx, SummaryKey(sessionID)).Result()
	if err != nil || summary == "" {
		return "", 0, false
	}
	index, err := RedisClient.Get(ctx, SummaryIndexKey(sessionID)).Int64()
	if err != nil || index <= 0 {
		return "", 0, false
	}
	return summary, index, true
}

// DeleteCompressedSummary 删除压缩摘要
func DeleteCompressedSummary(ctx context.Context, sessionID string) error {
	return RedisClient.Del(ctx, SummaryKey(sessionID), SummaryIndexKey(sessionID)).Err()
}

// SetUserProfile 写入用户画像热缓存
func SetUserProfile(ctx context.Context, userID, profileJSON string) error {
	return RedisClient.Set(ctx, UserProfileKey(userID), profileJSON, hotDataTTL).Err()
}

// GetUserProfile 读取用户画像；miss 返回 ("", false)
func GetUserProfile(ctx context.Context, userID string) (string, bool) {
	val, err := RedisClient.Get(ctx, UserProfileKey(userID)).Result()
	if err != nil || val == "" {
		return "", false
	}
	return val, true
}

// DeleteUserProfile 删除用户画像热缓存
func DeleteUserProfile(ctx context.Context, userID string) error {
	return RedisClient.Del(ctx, UserProfileKey(userID)).Err()
}

// SetAgentIdentity 写入 Agent 自我定位热缓存
func SetAgentIdentity(ctx context.Context, agentID uint, identityJSON string) error {
	return RedisClient.Set(ctx, AgentIdentityKey(agentID), identityJSON, hotDataTTL).Err()
}

// GetAgentIdentity 读取 Agent 自我定位；miss 返回 ("", false)
func GetAgentIdentity(ctx context.Context, agentID uint) (string, bool) {
	val, err := RedisClient.Get(ctx, AgentIdentityKey(agentID)).Result()
	if err != nil || val == "" {
		return "", false
	}
	return val, true
}
