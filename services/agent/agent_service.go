package agent

import (
	"ai-meeting/clients"
	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/pkg/ecode"

	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type AgentConversationService struct {
	repo agentRepository
}

func (s *AgentConversationService) repository() agentRepository {
	if s.repo != nil {
		return s.repo
	}
	return mysqlAgentRepository{}
}

// CreateConversationWithTitle 创建Agent会话并生成标题
// agentID 可选: 空默认 1(通用智能体), 非空则校验配置存在且启用
func (s *AgentConversationService) CreateConversationWithTitle(username, agentID string, firstMessage string) (*dto.AgentSessionCreateRespDTO, error) {
	agentIDUint, err := resolveAgentID(agentID)
	if err != nil {
		return nil, err
	}

	sessionID := uuid.New().String()
	title := "New Conversation"
	if len(firstMessage) > 50 {
		title = firstMessage[:50] + "..."
	}
	conversation := models.Conversation{
		SessionID: sessionID, UserID: username, Scene: models.ConversationSceneAgent,
		AgentID: agentIDUint, Title: title, Status: models.ConversationStatusActive, MessageCount: 0,
	}
	if err := s.repository().CreateConversation(&conversation); err != nil {
		return nil, err
	}
	return &dto.AgentSessionCreateRespDTO{SessionID: sessionID, Title: title}, nil
}

// resolveAgentID 解析并校验 agentID: 空默认 1, 必须存在且启用
func resolveAgentID(agentID string) (uint, error) {
	if strings.TrimSpace(agentID) == "" {
		agentID = "1"
	}
	id, err := strconv.ParseUint(agentID, 10, 32)
	if err != nil {
		return 0, ecode.New(ecode.ErrAgentPropertyNotFound, "invalid agent id: "+agentID)
	}
	if GetAgentPropertiesLoader().GetByAgentID(uint(id)) == nil {
		return 0, ecode.New(ecode.ErrAgentPropertyNotFound, "agent not found or disabled: "+agentID)
	}
	return uint(id), nil
}

// PageConversations 分页查询用户的Agent会话列表
func (s *AgentConversationService) PageConversations(username string, req dto.AgentConversationPageReqDTO) ([]models.AgentConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	convs, total, err := s.repository().PageConversations(username, models.ConversationSceneAgent, offset, req.Size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.AgentConversation, 0, len(convs))
	for _, c := range convs {
		out = append(out, toAgentConversation(c))
	}
	return out, total, nil
}

// EndConversation 结束会话
func (s *AgentConversationService) EndConversation(sessionID, userID string) error {
	return s.repository().EndConversation(sessionID, userID)
}

var (
	agentConversationServiceInstance *AgentConversationService
	agentConversationServiceOnce     sync.Once
)

// GetAgentConversationService 获取AgentConversationService单例
func GetAgentConversationService() *AgentConversationService {
	agentConversationServiceOnce.Do(func() {
		agentConversationServiceInstance = &AgentConversationService{repo: mysqlAgentRepository{}}
		logrus.Info("AgentConversationService instance created")
	})
	return agentConversationServiceInstance
}

type AgentMessageService struct {
	repo agentRepository
}

func (s *AgentMessageService) repository() agentRepository {
	if s.repo != nil {
		return s.repo
	}
	return mysqlAgentRepository{}
}

// GetConversationHistory 获取会话历史消息
func (s *AgentMessageService) GetConversationHistory(sessionID, userID string) ([]models.AgentMessage, error) {
	messages, err := s.repository().ListMessagesAsc(sessionID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.AgentMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, toAgentMessage(m))
	}
	return out, nil
}

// PageHistoryMessages 分页查询历史消息
func (s *AgentMessageService) PageHistoryMessages(sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	messages, total, err := s.repository().PageMessages(sessionID, userID, page, size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.AgentMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, toAgentMessage(m))
	}
	return out, total, nil
}

// SaveMessage 保存消息
func (s *AgentMessageService) SaveMessage(sessionID, userID, role, content string) error {
	return s.repository().CreateMessage(&models.Message{
		SessionID: sessionID, UserID: userID, Role: role, Content: content,
	})
}

// GetConversationHistoryWithContext 获取带上下文的历史消息
// 已移除: agent 侧不再使用压缩记忆,上下文未来由状态机结构化状态管理。

// AgentChatSSE Agent SSE 流式聊天
// onChunk 回调用于实时推送 chunk 给前端
// 返回完整回复内容
func (s *AgentMessageService) AgentChatSSE(sessionID, userID, content string, onChunk func(chunk string)) (string, error) {
	// 1. 会话归属校验
	conv, err := s.repository().FindConversationBySessionAndUser(sessionID, userID)
	if err != nil || conv == nil {
		return "", ecode.New(ecode.ErrSessionNotFound, "会话不存在或无权限")
	}

	// 2. 解析智能体配置（用于场景绑定,不再需要星辰凭证）
	agentProps := GetAgentPropertiesLoader().GetByAgentID(conv.AgentID)
	if agentProps == nil {
		return "", ecode.New(ecode.ErrAgentPropertyNotFound, fmt.Sprintf("智能体配置不存在, agentID=%d", conv.AgentID))
	}

	// 3. 保存用户消息
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.repository().CreateMessage(&models.Message{
		SessionID: sessionID, UserID: userID, Role: "user", Content: content,
	}); err != nil {
		return "", ecode.Wrap(err, "保存用户消息失败")
	}

	// 4. 加载历史消息构建 prompt messages
	rawMessages, err := s.repository().ListMessagesAsc(sessionID, userID)
	if err != nil {
		return "", ecode.Wrap(err, "加载历史消息失败")
	}
	messages := make([]models.AgentMessage, 0, len(rawMessages))
	for _, m := range rawMessages {
		messages = append(messages, toAgentMessage(m))
	}
	promptMessages := buildAgentPromptMessages(agentProps, messages, content)

	// 5. 调用 DeepSeek 流式聊天（aiID=0 走 config.ai.deepseek fallback）
	startTime := time.Now()
	var fullContent strings.Builder
	err = clients.CallConfiguredAIChatStream(ctx, 0, promptMessages, 0.7, func(chunk clients.ChatStreamChunk) error {
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			if onChunk != nil {
				onChunk(chunk.Content)
			}
		}
		return nil
	})
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		// 6a. 出错也保存一条错误 assistant 消息
		meta, _ := json.Marshal(map[string]interface{}{"response_time": responseTime, "error": err.Error()})
		_ = s.repository().CreateMessage(&models.Message{
			SessionID: sessionID, UserID: userID, Role: "assistant",
			Content: "Sorry, an error occurred while processing your request.", Metadata: string(meta),
		})
		logrus.Errorf("Agent chat failed, session=%s, err=%v", sessionID, err)
		return "", ecode.New(ecode.ErrDeepSeekCall, "调用 DeepSeek 失败")
	}

	// 6b. 保存 assistant 回复
	meta, _ := json.Marshal(map[string]interface{}{"response_time": responseTime})
	if err := s.repository().CreateMessage(&models.Message{
		SessionID: sessionID, UserID: userID, Role: "assistant",
		Content: fullContent.String(), Metadata: string(meta),
	}); err != nil {
		logrus.Errorf("Failed to save assistant message, session=%s, err=%v", sessionID, err)
	}

	// 7. 更新会话消息计数
	if total, err := s.repository().CountMessages(sessionID, userID); err == nil {
		if err := s.repository().UpdateConversationMessageCount(sessionID, userID, int(total)); err != nil {
			logrus.Errorf("Failed to update conversation message count, session=%s, err=%v", sessionID, err)
		}
	}

	logrus.Infof("Agent chat completed, session=%s, responseTime=%dms, contentLen=%d",
		sessionID, responseTime, fullContent.Len())
	return fullContent.String(), nil
}

// buildAgentPromptMessages 构建 DeepSeek 的 prompt messages
// 包含 system prompt（智能体描述）+ 历史对话 + 当前用户消息
func buildAgentPromptMessages(agentProps *models.AgentProperties, history []models.AgentMessage, currentContent string) []clients.PromptMessage {
	messages := []clients.PromptMessage{
		{
			Role: "system",
			Content: fmt.Sprintf("你是一个智能助手。%s。根据可用历史上下文回答用户当前问题。",
				agentProps.Description),
		},
	}

	// 历史消息（排除最后一条当前用户消息,它作为 user message 单独传）
	if len(history) > 0 {
		history = history[:len(history)-1]
	}
	for _, msg := range history {
		role := "user"
		if msg.Role == "assistant" {
			role = "assistant"
		}
		messages = append(messages, clients.PromptMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	messages = append(messages, clients.PromptMessage{
		Role:    "user",
		Content: currentContent,
	})
	return messages
}

var (
	agentMessageServiceInstance *AgentMessageService
	agentMessageServiceOnce     sync.Once
)

// GetAgentMessageService 获取AgentMessageService单例
func GetAgentMessageService() *AgentMessageService {
	agentMessageServiceOnce.Do(func() {
		agentMessageServiceInstance = &AgentMessageService{repo: mysqlAgentRepository{}}
		logrus.Info("AgentMessageService instance created")
	})
	return agentMessageServiceInstance
}

type AgentPropertiesService struct {
	repo agentRepository
}

func (s *AgentPropertiesService) repository() agentRepository {
	if s.repo != nil {
		return s.repo
	}
	return mysqlAgentRepository{}
}

// Create 创建Agent配置
func (s *AgentPropertiesService) Create(req dto.AgentPropertiesReqDTO) error {
	prop := models.AgentProperties{Name: req.Name, Description: req.Description, Config: req.Config, IsEnabled: true}
	return s.repository().CreateAgentProperties(&prop)
}

// Update 更新Agent配置
func (s *AgentPropertiesService) Update(req dto.AgentPropertiesReqDTO) error {
	return s.repository().UpdateAgentProperties(req.ID, map[string]interface{}{"name": req.Name, "description": req.Description, "config": req.Config})
}

// Delete 删除Agent配置
func (s *AgentPropertiesService) Delete(id uint) error {
	return s.repository().DeleteAgentProperties(id)
}

// GetByName 根据名称查询Agent配置
func (s *AgentPropertiesService) GetByName(name string) (*models.AgentProperties, error) {
	return s.repository().FindAgentPropertiesByName(name)
}

// GetByPage 分页查询Agent配置
func (s *AgentPropertiesService) GetByPage(req dto.AgentPropertiesReqDTO) ([]models.AgentProperties, int64, error) {
	return s.repository().PageAgentProperties(10)
}

var (
	agentPropertiesServiceInstance *AgentPropertiesService
	agentPropertiesServiceOnce     sync.Once
)

// GetAgentPropertiesService 获取AgentPropertiesService单例
func GetAgentPropertiesService() *AgentPropertiesService {
	agentPropertiesServiceOnce.Do(func() {
		agentPropertiesServiceInstance = &AgentPropertiesService{repo: mysqlAgentRepository{}}
		logrus.Info("AgentPropertiesService instance created")
	})
	return agentPropertiesServiceInstance
}

type AgentFileAssetService struct {
	repo agentRepository
}

func (s *AgentFileAssetService) repository() agentRepository {
	if s.repo != nil {
		return s.repo
	}
	return mysqlAgentRepository{}
}

// UploadAndPersist 上传文件并持久化记录
func (s *AgentFileAssetService) UploadAndPersist(sessionID, bizType, username string, filename, path string, size int64) (*dto.AgentFileUploadRespDTO, error) {
	asset := models.AgentFileAsset{SessionID: sessionID, UserID: username, Filename: filename, Path: path, BizType: bizType, Size: size}
	if err := s.repository().CreateAgentFileAsset(&asset); err != nil {
		return nil, err
	}
	return &dto.AgentFileUploadRespDTO{FileID: fmt.Sprintf("%d", asset.ID), Filename: filename, Path: path}, nil
}

var (
	agentFileAssetServiceInstance *AgentFileAssetService
	agentFileAssetServiceOnce     sync.Once
)

// GetAgentFileAssetService 获取AgentFileAssetService单例
func GetAgentFileAssetService() *AgentFileAssetService {
	agentFileAssetServiceOnce.Do(func() {
		agentFileAssetServiceInstance = &AgentFileAssetService{repo: mysqlAgentRepository{}}
		logrus.Info("AgentFileAssetService instance created")
	})
	return agentFileAssetServiceInstance
}

// toAgentConversation 统一会话模型转旧响应模型
func toAgentConversation(c models.Conversation) models.AgentConversation {
	return models.AgentConversation{
		SessionID: c.SessionID, UserID: c.UserID, AgentID: c.AgentID,
		Title: c.Title, Status: c.Status, MessageCnt: c.MessageCount,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// toAgentMessage 统一消息模型转旧响应模型
func toAgentMessage(m models.Message) models.AgentMessage {
	return models.AgentMessage{
		ID: m.ID, SessionID: m.SessionID, UserID: m.UserID, Role: m.Role,
		Content: m.Content, Sequence: int(m.Sequence), CreatedAt: m.CreatedAt,
	}
}
