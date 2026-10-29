package run

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
)

// runRepository 是 run 服务需要的最小持久化接口。
type runRepository interface {
	CreateAgentRun(run *models.AgentRun) error
	FinishAgentRun(runID, status, stopReason, output, errMsg string, steps, tokens int) error
	UpdateAgentRunPlan(runID, planJSON string) error
	UpdateAgentRunContextReport(runID, reportJSON string) error
	CreateAgentStep(step *models.AgentStep) error

	CreateMessage(msg *models.Message) error
	CountMessages(sessionID, userID string) (int64, error)
	UpdateConversationMessageCount(sessionID, userID string, count int) error
	FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error)
	CreateConversation(conv *models.Conversation) error
}

type mysqlRunRepository struct{}

func (mysqlRunRepository) CreateAgentRun(run *models.AgentRun) error {
	return mysqlrepo.CreateAgentRun(run)
}

func (mysqlRunRepository) FinishAgentRun(runID, status, stopReason, output, errMsg string, steps, tokens int) error {
	return mysqlrepo.FinishAgentRun(runID, status, stopReason, output, errMsg, steps, tokens)
}

func (mysqlRunRepository) UpdateAgentRunPlan(runID, planJSON string) error {
	return mysqlrepo.UpdateAgentRunPlan(runID, planJSON)
}

func (mysqlRunRepository) UpdateAgentRunContextReport(runID, reportJSON string) error {
	return mysqlrepo.UpdateAgentRunContextReport(runID, reportJSON)
}

func (mysqlRunRepository) CreateAgentStep(step *models.AgentStep) error {
	return mysqlrepo.CreateAgentStep(step)
}

func (mysqlRunRepository) CreateMessage(msg *models.Message) error {
	return mysqlrepo.CreateMessage(msg)
}

func (mysqlRunRepository) CountMessages(sessionID, userID string) (int64, error) {
	return mysqlrepo.CountMessages(sessionID, userID)
}

func (mysqlRunRepository) UpdateConversationMessageCount(sessionID, userID string, count int) error {
	return mysqlrepo.UpdateConversationMessageCount(sessionID, userID, count)
}

func (mysqlRunRepository) FindConversationBySessionAndUser(sessionID, userID string) (*models.Conversation, error) {
	return mysqlrepo.FindConversationBySessionAndUser(sessionID, userID)
}

func (mysqlRunRepository) CreateConversation(conv *models.Conversation) error {
	return mysqlrepo.CreateConversation(conv)
}
