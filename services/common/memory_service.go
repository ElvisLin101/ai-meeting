package common

import (
	"ai-meeting/clients"
	"ai-meeting/models"
	"ai-meeting/pkg/singleflight"
	"ai-meeting/repositories"
	mongorepo "ai-meeting/repositories/mongo"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type MemoryService struct {
	mu        sync.RWMutex
	threshold int
}

var memoryServiceInstance *MemoryService

// GetMemoryService 获取 MemoryService 单例
func GetMemoryService() *MemoryService {
	if memoryServiceInstance == nil {
		memoryServiceInstance = &MemoryService{threshold: COMPRESSION_THRESHOLD}
	}
	return memoryServiceInstance
}

const (
	COMPRESSED_CONTEXT_INDEX_SUFFIX = "index"
	MIN_COMPRESSION_THRESHOLD       = 1024
	COMPRESSION_THRESHOLD           = 4096
	MAX_COMPRESSION_THRESHOLD       = 32768
	COMPRESSION_TRIGGER_OFFSET      = 500
	COMPRESSION_RATIO               = 0.8
	REDIS_EXPIRE_DURATION           = 7 * 24 * time.Hour
)

// GetContext 获取会话上下文。压缩摘要优先读 Redis，Redis 失效后从 MongoDB 恢复。
func (s *MemoryService) GetContext(sessionID, userID string, threshold int) (string, error) {
	threshold = s.normalizeThreshold(threshold)
	ctx := context.Background()

	compressedCtx, compressedIndex := s.loadCompressedContext(ctx, sessionID)

	messages, err := mongorepo.ListAgentMessagesAfterSequenceDesc(ctx, sessionID, userID, compressedIndex)
	if err != nil {
		return "", err
	}

	contextText, availableLength := s.buildContextWithWindow(compressedCtx, messages, threshold)
	if availableLength >= threshold-COMPRESSION_TRIGGER_OFFSET {
		s.CompressContext(sessionID, userID, threshold)
	}

	return contextText, nil
}

// loadCompressedContext 加载 Agent 压缩上下文（Redis 优先，miss 从 Mongo 恢复并异步回填）
func (s *MemoryService) loadCompressedContext(ctx context.Context, sessionID string) (string, int) {
	compressedCtx, index, ok := s.getCompressedContextFromRedis(ctx, sessionID)
	if ok {
		return compressedCtx, index
	}

	mongoCtx, err := s.getCompressedContextFromMongo(sessionID)
	if err != nil {
		logrus.Warnf("Failed to restore compressed context from MongoDB, session=%s, err=%v", sessionID, err)
		return "", 0
	}
	if mongoCtx == nil || mongoCtx.CompressedContent == "" || mongoCtx.Index <= 0 {
		return "", 0
	}

	go s.syncToRedis(sessionID, mongoCtx.CompressedContent, mongoCtx.Index)
	return mongoCtx.CompressedContent, mongoCtx.Index
}

// getCompressedContextFromRedis 从 Redis 读取 Agent 压缩摘要和索引
func (s *MemoryService) getCompressedContextFromRedis(ctx context.Context, sessionID string) (string, int, bool) {
	compressedCtx, err := repositories.RedisClient.Get(ctx, compressedContextKey(sessionID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		logrus.Warnf("Failed to read compressed context from Redis, session=%s, err=%v", sessionID, err)
		return "", 0, false
	}
	if compressedCtx == "" {
		return "", 0, false
	}

	indexStr, err := repositories.RedisClient.Get(ctx, compressedContextIndexKey(sessionID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		logrus.Warnf("Failed to read compressed index from Redis, session=%s, err=%v", sessionID, err)
		return "", 0, false
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil || index <= 0 {
		return "", 0, false
	}

	return compressedCtx, index, true
}

// buildContextWithWindow 用预算窗口拼接压缩摘要 + 近期 Agent 消息，超预算时截断，返回上下文文本和可用长度
func (s *MemoryService) buildContextWithWindow(compressedCtx string, messages []models.AgentMessage, threshold int) (string, int) {
	var contextBuilder strings.Builder

	baseLength := 0
	if compressedCtx != "" {
		contextBuilder.WriteString("【历史压缩摘要】\n")
		contextBuilder.WriteString(compressedCtx)
		contextBuilder.WriteString("\n--- 以下为未压缩的近期对话 ---\n")
		baseLength = contextBuilder.Len()
	}

	availableLength := baseLength + messagesLength(messages)
	windowBudget := threshold - COMPRESSION_TRIGGER_OFFSET - baseLength
	if windowBudget < 0 {
		windowBudget = 0
	}

	windowMsgs := make([]models.AgentMessage, 0, len(messages))
	windowLength := 0
	for _, msg := range messages {
		lineLength := len(formatMessageLine(msg))
		if windowLength+lineLength > windowBudget {
			break
		}
		windowMsgs = append(windowMsgs, msg)
		windowLength += lineLength
	}

	// Mongo 按 sequence 倒序取窗口，写入上下文时恢复为正序，避免打乱对话语义。
	for i := len(windowMsgs) - 1; i >= 0; i-- {
		contextBuilder.WriteString(formatMessageLine(windowMsgs[i]))
	}

	return contextBuilder.String(), availableLength
}

// CompressContext 异步压缩上下文：只压缩旧的 80%，新的 20% 继续由 index 后的 Mongo 消息补齐。
func (s *MemoryService) CompressContext(sessionID, userID string, threshold int) {
	threshold = s.normalizeThreshold(threshold)

	// 使用分布式 singleflight 去重，全集群同一 session 只压缩一次
	go func() {
		sfKey := "compress:agent:" + sessionID + ":" + userID
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		result, err := repositories.SingleFlight.Do(ctx, sfKey, func(ctx context.Context, writer *singleflight.StreamWriter) (interface{}, error) {
			return s.doCompressContext(ctx, writer, sessionID, userID, threshold)
		})
		if err != nil {
			logrus.Errorf("Context compression singleflight failed for session %s: %v", sessionID, err)
			return
		}
		_ = result
	}()
}

// doCompressContext 执行实际的上下文压缩逻辑
// writer 用于在流式压缩过程中刷新 singleflight 心跳进度。
func (s *MemoryService) doCompressContext(ctx context.Context, writer *singleflight.StreamWriter, sessionID, userID string, threshold int) (interface{}, error) {
	messages, err := mongorepo.ListAgentMessagesDesc(ctx, sessionID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	if len(messages) == 0 {
		return nil, nil
	}

	totalLength := messagesLength(messages)
	if totalLength < threshold-COMPRESSION_TRIGGER_OFFSET {
		return nil, nil
	}

	recentCount := int(float64(len(messages)) * (1 - COMPRESSION_RATIO))
	if recentCount < 1 {
		recentCount = 1
	}
	if recentCount >= len(messages) {
		recentCount = len(messages) - 1
	}

	compressMessages := messages[recentCount:]
	if len(compressMessages) == 0 {
		return nil, nil
	}

	contextToCompress := buildChronologicalContext(compressMessages)
	compressedText, err := s.callAIForCompression(ctx, writer, contextToCompress)
	if err != nil {
		return nil, fmt.Errorf("failed to compress context: %w", err)
	}

	compressIndex := compressMessages[0].Sequence
	if err := s.saveCompressedContextToRedis(ctx, sessionID, compressedText, compressIndex); err != nil {
		return nil, fmt.Errorf("failed to save compressed context to Redis: %w", err)
	}

	go s.persistToMongo(sessionID, compressedText, compressIndex, totalLength, len(messages))

	logrus.Infof("Context compressed for session %s, index: %d, recent count: %d",
		sessionID, compressIndex, recentCount)
	return nil, nil
}

// saveCompressedContextToRedis 将 Agent 压缩摘要和索引写入 Redis（TTL 7 天）
func (s *MemoryService) saveCompressedContextToRedis(ctx context.Context, sessionID, compressedContent string, index int) error {
	if err := repositories.RedisClient.Set(ctx, compressedContextKey(sessionID), compressedContent, REDIS_EXPIRE_DURATION).Err(); err != nil {
		return err
	}
	return repositories.RedisClient.Set(ctx, compressedContextIndexKey(sessionID), strconv.Itoa(index), REDIS_EXPIRE_DURATION).Err()
}

// persistToMongo 异步将 Agent 压缩上下文持久化到 MongoDB（upsert）
func (s *MemoryService) persistToMongo(sessionID, compressedContent string, index, totalLength, messageCount int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := mongorepo.UpsertCompressedContext(ctx, mongorepo.CompressedContextUpsert{
		ID:                sessionID,
		SessionID:         sessionID,
		CompressedContent: compressedContent,
		Index:             index,
		TotalLength:       totalLength,
		MessageCount:      messageCount,
	})
	if err != nil {
		logrus.Error("Failed to persist compressed context to MongoDB:", err)
		return
	}
	logrus.Infof("Compressed context persisted to MongoDB for session %s", sessionID)
}

// getCompressedContextFromMongo 从 MongoDB 读取 Agent 压缩上下文
func (s *MemoryService) getCompressedContextFromMongo(sessionID string) (*models.CompressedContext, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := mongorepo.FindCompressedContextByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	// 兼容旧版本按 session_id 写入的压缩快照。
	return mongorepo.FindCompressedContextBySessionID(ctx, sessionID)
}

// syncToRedis 异步将 Mongo 中的 Agent 压缩上下文同步到 Redis
func (s *MemoryService) syncToRedis(sessionID, compressedContent string, index int) {
	ctx := context.Background()
	if err := s.saveCompressedContextToRedis(ctx, sessionID, compressedContent, index); err != nil {
		logrus.Error("Failed to sync compressed context to Redis:", err)
		return
	}
	logrus.Infof("Compressed context synced to Redis for session %s", sessionID)
}

// callAIForCompression 调用 AI 流式生成 Agent 上下文摘要，每段 chunk 刷新 singleflight 心跳；失败时回退本地截断
func (s *MemoryService) callAIForCompression(ctx context.Context, writer *singleflight.StreamWriter, contextText string) (string, error) {
	prompt := buildCompressionPrompt(contextText)

	var replyBuilder strings.Builder
	err := clients.CallConfiguredAIChatStream(ctx, 0, []clients.PromptMessage{
		{Role: "system", Content: "你是一个会话记忆压缩器。只输出压缩后的中文记忆摘要，不要解释过程。"},
		{Role: "user", Content: prompt},
	}, 0.2, func(chunk clients.ChatStreamChunk) error {
		// 累积完整压缩结果用于落库
		if chunk.Content != "" {
			replyBuilder.WriteString(chunk.Content)
		}
		// 每收到一段输出就刷新 progressKey 作心跳，让 follower 知道 leader 还在干活
		if _, werr := writer.Write([]byte(chunk.Content)); werr != nil {
			return werr
		}
		return nil
	})
	if err == nil {
		if compressed := strings.TrimSpace(replyBuilder.String()); compressed != "" {
			return compressed, nil
		}
	}
	if err != nil {
		logrus.Warnf("AI compression stream request failed, falling back to local summary: %v", err)
	}
	return fallbackCompressedSummary(contextText), nil
}

// buildCompressionPrompt 构建 Agent 压缩请求的 system prompt
func buildCompressionPrompt(contextText string) string {
	return fmt.Sprintf(`请将下面的多轮聊天记录压缩为“长期记忆摘要”，用于下一次继续对话时恢复上下文。

要求：
1. 保留用户目标、偏好、关键事实、已经达成的结论、待办事项和重要约束。
2. 保留会影响后续回答的代码文件、接口、字段、错误、决策和未完成问题。
3. 删除寒暄、重复表达、无关细节和已经被后文推翻的信息。
4. 如果存在冲突信息，请保留最新结论，并简要标注旧信息已被覆盖。
5. 输出中文，控制在 500 字以内。
6. 只输出摘要正文，使用“【长期记忆】”开头。

--- 待压缩聊天记录开始 ---
%s
--- 待压缩聊天记录结束 ---`, contextText)
}

// fallbackCompressedSummary AI 压缩失败时的本地兜底：截取首尾 450 字
func fallbackCompressedSummary(contextText string) string {
	contextText = strings.TrimSpace(contextText)
	const maxLen = 900
	if len(contextText) <= maxLen {
		return "【长期记忆】" + contextText
	}
	head := contextText[:450]
	tail := contextText[len(contextText)-450:]
	return "【长期记忆】" + head + "\n...\n" + tail
}

// buildChronologicalContext 将 Agent 消息列表按时间正序拼接为文本
func buildChronologicalContext(messages []models.AgentMessage) string {
	var builder strings.Builder
	for i := len(messages) - 1; i >= 0; i-- {
		builder.WriteString(formatMessageLine(messages[i]))
	}
	return builder.String()
}

// messagesLength 计算 Agent 消息列表总字节长度
func messagesLength(messages []models.AgentMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(formatMessageLine(msg))
	}
	return total
}

// formatMessageLine 格式化单条 Agent 消息为文本行
func formatMessageLine(msg models.AgentMessage) string {
	role := "assistant"
	if msg.Role == "user" {
		role = "user"
	}
	return fmt.Sprintf("%s: %s\n", role, msg.Content)
}

// ClearCompressedContext 清理 Agent 压缩上下文（Redis + Mongo）
func (s *MemoryService) ClearCompressedContext(sessionID string) error {
	ctx := context.Background()
	_, err1 := repositories.RedisClient.Del(ctx, compressedContextKey(sessionID)).Result()
	_, err2 := repositories.RedisClient.Del(ctx, compressedContextIndexKey(sessionID)).Result()

	go s.clearFromMongo(sessionID)

	if err1 != nil {
		return err1
	}
	return err2
}

// clearFromMongo 异步清理 MongoDB 中的 Agent 压缩上下文
func (s *MemoryService) clearFromMongo(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mongorepo.DeleteCompressedContextByID(ctx, sessionID); err != nil {
		logrus.Error("Failed to delete compressed context from MongoDB:", err)
	}
}

// GetCompressionThreshold 获取当前 Agent 压缩阈值
func (s *MemoryService) GetCompressionThreshold() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.threshold == 0 {
		return COMPRESSION_THRESHOLD
	}
	return s.threshold
}

// SetCompressionThreshold 设置 Agent 压缩阈值（带范围校验 1024~32768）
func (s *MemoryService) SetCompressionThreshold(threshold int) error {
	if threshold < MIN_COMPRESSION_THRESHOLD || threshold > MAX_COMPRESSION_THRESHOLD {
		return fmt.Errorf("threshold must be between %d and %d", MIN_COMPRESSION_THRESHOLD, MAX_COMPRESSION_THRESHOLD)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.threshold = threshold
	return nil
}

// GetCompressionThresholdConfig 返回 Agent 阈值配置（当前值、最小值、最大值、触发偏移）
func (s *MemoryService) GetCompressionThresholdConfig() (int, int, int, int) {
	return s.GetCompressionThreshold(), MIN_COMPRESSION_THRESHOLD, MAX_COMPRESSION_THRESHOLD, COMPRESSION_TRIGGER_OFFSET
}

// normalizeThreshold 规范化阈值到合法范围
func (s *MemoryService) normalizeThreshold(threshold int) int {
	if threshold == 0 {
		return s.GetCompressionThreshold()
	}
	if threshold < MIN_COMPRESSION_THRESHOLD {
		return MIN_COMPRESSION_THRESHOLD
	}
	if threshold > MAX_COMPRESSION_THRESHOLD {
		return MAX_COMPRESSION_THRESHOLD
	}
	return threshold
}

// compressedContextKey 生成 Agent 压缩摘要的 Redis key（直接用 sessionId）
func compressedContextKey(sessionID string) string {
	return sessionID
}

// compressedContextIndexKey 生成 Agent 压缩索引的 Redis key（sessionId + "index"）
func compressedContextIndexKey(sessionID string) string {
	return sessionID + COMPRESSED_CONTEXT_INDEX_SUFFIX
}
