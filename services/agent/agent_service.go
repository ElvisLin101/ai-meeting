package agent

import (
	"ai-meeting/clients"
	"ai-meeting/dto"
	"ai-meeting/models"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type AgentConversationService struct{}

// CreateConversationWithTitle 创建Agent会话并生成标题
func (s *AgentConversationService) CreateConversationWithTitle(username, agentID string, firstMessage string) (*dto.AgentSessionCreateRespDTO, error) {
	sessionID := uuid.New().String()
	title := "New Conversation"
	if len(firstMessage) > 50 {
		title = firstMessage[:50] + "..."
	}
	conversation := models.AgentConversation{SessionID: sessionID, UserID: username, AgentID: 1, Title: title, Status: 1, MessageCnt: 0}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mongorepo.CreateAgentConversation(ctx, &conversation); err != nil {
		return nil, err
	}
	return &dto.AgentSessionCreateRespDTO{SessionID: sessionID, Title: title}, nil
}

// PageConversations 分页查询用户的Agent会话列表
func (s *AgentConversationService) PageConversations(username string, req dto.AgentConversationPageReqDTO) ([]models.AgentConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.PageAgentConversations(ctx, username, offset, req.Size)
}

// EndConversation 结束会话
func (s *AgentConversationService) EndConversation(sessionID, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.EndAgentConversation(ctx, sessionID, userID)
}

var agentConversationServiceInstance *AgentConversationService

// GetAgentConversationService 获取AgentConversationService单例
func GetAgentConversationService() *AgentConversationService {
	if agentConversationServiceInstance == nil {
		agentConversationServiceInstance = &AgentConversationService{}
	}
	logrus.Info("AgentConversationService instance created")
	return agentConversationServiceInstance
}

type AgentMessageService struct{}

// GetConversationHistory 获取会话历史消息
func (s *AgentMessageService) GetConversationHistory(sessionID, userID string) ([]models.AgentMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return mongorepo.ListAgentMessagesAsc(ctx, sessionID, userID)
}

// PageHistoryMessages 分页查询历史消息
func (s *AgentMessageService) PageHistoryMessages(sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return mongorepo.PageAgentMessages(ctx, sessionID, page, size, userID)
}

// SaveMessage 保存消息
func (s *AgentMessageService) SaveMessage(sessionID, userID, role, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return mongorepo.SaveAgentMessage(ctx, sessionID, userID, role, content)
}

// GetConversationHistoryWithContext 获取带上下文的历史消息
// 已移除: agent 侧不再使用压缩记忆,上下文未来由状态机结构化状态管理。

// AgentChatSSE Agent SSE 流式聊天
// onChunk 回调用于实时推送 chunk 给前端
// 返回完整回复内容
func (s *AgentMessageService) AgentChatSSE(sessionID, userID, content string, onChunk func(chunk string)) (string, error) {
	// 1. 会话归属校验
	convCtx, convCancel := context.WithTimeout(context.Background(), 5*time.Second)
	conv, err := mongorepo.GetAgentConversationBySessionId(convCtx, sessionID, userID)
	convCancel()
	if err != nil || conv == nil {
		return "", fmt.Errorf("会话不存在或无权限: %w", err)
	}

	// 2. 解析智能体配置（用于场景绑定,不再需要星辰凭证）
	agentProps := GetAgentPropertiesLoader().GetByAgentID(conv.AgentID)
	if agentProps == nil {
		return "", fmt.Errorf("智能体配置不存在, agentID=%d", conv.AgentID)
	}

	// 3. 保存用户消息
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mongorepo.SaveAgentMessage(ctx, sessionID, userID, "user", content); err != nil {
		return "", fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 4. 加载历史消息构建 prompt messages
	historyCtx, historyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer historyCancel()
	messages, err := mongorepo.ListAgentMessagesAsc(historyCtx, sessionID, userID)
	if err != nil {
		return "", fmt.Errorf("加载历史消息失败: %w", err)
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
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer saveCancel()
		mongorepo.SaveAgentMessageWithDetail(saveCtx, sessionID, userID, "assistant", "Sorry, an error occurred while processing your request.", responseTime, err.Error())
		logrus.Errorf("Agent chat failed, session=%s, err=%v", sessionID, err)
		return "", fmt.Errorf("调用 DeepSeek 失败: %w", err)
	}

	// 6b. 保存 assistant 回复
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	assistantSeq, err := mongorepo.SaveAgentMessageWithDetail(saveCtx, sessionID, userID, "assistant", fullContent.String(), responseTime, "")
	if err != nil {
		logrus.Errorf("Failed to save assistant message, session=%s, err=%v", sessionID, err)
	}

	// 7. 更新会话消息计数
	if err := mongorepo.UpdateAgentConversationMessageCount(saveCtx, sessionID, assistantSeq); err != nil {
		logrus.Errorf("Failed to update conversation message count, session=%s, err=%v", sessionID, err)
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

var agentMessageServiceInstance *AgentMessageService

// GetAgentMessageService 获取AgentMessageService单例
func GetAgentMessageService() *AgentMessageService {
	if agentMessageServiceInstance == nil {
		agentMessageServiceInstance = &AgentMessageService{}
	}
	logrus.Info("AgentMessageService instance created")
	return agentMessageServiceInstance
}

type AgentPropertiesService struct{}

// Create 创建Agent配置
func (s *AgentPropertiesService) Create(req dto.AgentPropertiesReqDTO) error {
	prop := models.AgentProperties{Name: req.Name, Description: req.Description, Config: req.Config, IsEnabled: true}
	return mysqlrepo.CreateAgentProperties(&prop)
}

// Update 更新Agent配置
func (s *AgentPropertiesService) Update(req dto.AgentPropertiesReqDTO) error {
	return mysqlrepo.UpdateAgentProperties(req.ID, map[string]interface{}{"name": req.Name, "description": req.Description, "config": req.Config})
}

// Delete 删除Agent配置
func (s *AgentPropertiesService) Delete(id uint) error {
	return mysqlrepo.DeleteAgentProperties(id)
}

// GetByName 根据名称查询Agent配置
func (s *AgentPropertiesService) GetByName(name string) (*models.AgentProperties, error) {
	return mysqlrepo.FindAgentPropertiesByName(name)
}

// GetByPage 分页查询Agent配置
func (s *AgentPropertiesService) GetByPage(req dto.AgentPropertiesReqDTO) ([]models.AgentProperties, int64, error) {
	return mysqlrepo.PageAgentProperties(10)
}

var agentPropertiesServiceInstance *AgentPropertiesService

// GetAgentPropertiesService 获取AgentPropertiesService单例
func GetAgentPropertiesService() *AgentPropertiesService {
	if agentPropertiesServiceInstance == nil {
		agentPropertiesServiceInstance = &AgentPropertiesService{}
	}
	logrus.Info("AgentPropertiesService instance created")
	return agentPropertiesServiceInstance
}

type AgentFileAssetService struct{}

// UploadAndPersist 上传文件并持久化记录
func (s *AgentFileAssetService) UploadAndPersist(sessionID, bizType, username string, filename, path string, size int64) (*dto.AgentFileUploadRespDTO, error) {
	asset := models.AgentFileAsset{SessionID: sessionID, UserID: username, Filename: filename, Path: path, BizType: bizType, Size: size}
	if err := mysqlrepo.CreateAgentFileAsset(&asset); err != nil {
		return nil, err
	}
	return &dto.AgentFileUploadRespDTO{FileID: fmt.Sprintf("%d", asset.ID), Filename: filename, Path: path}, nil
}

var agentFileAssetServiceInstance *AgentFileAssetService

// GetAgentFileAssetService 获取AgentFileAssetService单例
func GetAgentFileAssetService() *AgentFileAssetService {
	if agentFileAssetServiceInstance == nil {
		agentFileAssetServiceInstance = &AgentFileAssetService{}
	}
	logrus.Info("AgentFileAssetService instance created")
	return agentFileAssetServiceInstance
}
