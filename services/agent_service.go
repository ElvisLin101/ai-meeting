package services

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	mongorepo "ai-meeting/repositories/mongo"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
	"fmt"
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
	if err := mysqlrepo.CreateAgentConversation(&conversation); err != nil {
		return nil, err
	}
	return &dto.AgentSessionCreateRespDTO{SessionID: sessionID, Title: title}, nil
}

// PageConversations 分页查询用户的Agent会话列表
func (s *AgentConversationService) PageConversations(username string, req dto.AgentConversationPageReqDTO) ([]models.AgentConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	return mysqlrepo.PageAgentConversations(username, offset, req.Size)
}

// EndConversation 结束会话
func (s *AgentConversationService) EndConversation(sessionID, userID string) error {
	return mysqlrepo.EndAgentConversation(sessionID, userID)
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
func (s *AgentMessageService) GetConversationHistoryWithContext(sessionID, userID string) (string, error) {
	memoryService := GetMemoryService()
	threshold := memoryService.GetCompressionThreshold()

	// 获取上下文
	context, err := memoryService.GetContext(sessionID, userID, threshold)
	if err != nil {
		return "", err
	}

	return context, nil
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
