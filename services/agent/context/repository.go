package agentctx

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"sync"
)

// repository 是 agent 上下文/画像/压缩需要的 MySQL 存储接口。
// 生产实现走 mysqlrepo；测试可替换为 fake。
type repository interface {
	ListRecentMessagesAfter(sessionID, userID string, afterSeq int64, limit int) ([]models.Message, error)
	ListMessagesDesc(sessionID, userID string) ([]models.Message, error)

	FindCompressedContext(sessionID string) (*models.CompressedContextRecord, error)
	UpsertCompressedContext(record *models.CompressedContextRecord) error

	FindUserProfile(userID string) (*models.UserProfile, error)
	UpsertUserProfile(profile *models.UserProfile) error
	UpdateUserProfileCAS(userID, profileJSON string, expectedVersion int, lastSeq int64) (bool, error)

	FindAgentIdentity(agentID uint) (*models.AgentIdentity, error)
	UpsertAgentIdentity(identity *models.AgentIdentity) error

	ListPendingProfileCandidates(userID string, limit int) ([]models.ProfileCandidate, error)
	CreateProfileCandidate(c *models.ProfileCandidate) error
	MarkProfileCandidates(userID string, ids []uint, status string) error
	CountUserMessagesAfter(sessionID, userID string, afterSeq int64) (int64, error)

	CreateProfileConfirmation(c *models.ProfileConfirmation) error
	ListPendingProfileConfirmations(userID string, limit int) ([]models.ProfileConfirmation, error)
	FindProfileConfirmation(id uint, userID string) (*models.ProfileConfirmation, error)
	MarkProfileConfirmation(id uint, userID, status string) (bool, error)
	CreateProfileEvent(e *models.ProfileEvent) error
}

type mysqlRepository struct{}

func (mysqlRepository) ListRecentMessagesAfter(sessionID, userID string, afterSeq int64, limit int) ([]models.Message, error) {
	return mysqlrepo.ListRecentMessagesAfter(sessionID, userID, afterSeq, limit)
}

func (mysqlRepository) ListMessagesDesc(sessionID, userID string) ([]models.Message, error) {
	return mysqlrepo.ListMessagesDesc(sessionID, userID)
}

func (mysqlRepository) FindCompressedContext(sessionID string) (*models.CompressedContextRecord, error) {
	return mysqlrepo.FindCompressedContext(sessionID)
}

func (mysqlRepository) UpsertCompressedContext(record *models.CompressedContextRecord) error {
	return mysqlrepo.UpsertCompressedContext(record)
}

func (mysqlRepository) FindUserProfile(userID string) (*models.UserProfile, error) {
	return mysqlrepo.FindUserProfile(userID)
}

func (mysqlRepository) UpsertUserProfile(profile *models.UserProfile) error {
	return mysqlrepo.UpsertUserProfile(profile)
}

func (mysqlRepository) UpdateUserProfileCAS(userID, profileJSON string, expectedVersion int, lastSeq int64) (bool, error) {
	return mysqlrepo.UpdateUserProfileCAS(userID, profileJSON, expectedVersion, lastSeq)
}

func (mysqlRepository) FindAgentIdentity(agentID uint) (*models.AgentIdentity, error) {
	return mysqlrepo.FindAgentIdentity(agentID)
}

func (mysqlRepository) UpsertAgentIdentity(identity *models.AgentIdentity) error {
	return mysqlrepo.UpsertAgentIdentity(identity)
}

func (mysqlRepository) ListPendingProfileCandidates(userID string, limit int) ([]models.ProfileCandidate, error) {
	return mysqlrepo.ListPendingProfileCandidates(userID, limit)
}

func (mysqlRepository) CreateProfileCandidate(c *models.ProfileCandidate) error {
	return mysqlrepo.CreateProfileCandidate(c)
}

func (mysqlRepository) MarkProfileCandidates(userID string, ids []uint, status string) error {
	return mysqlrepo.MarkProfileCandidates(userID, ids, status)
}

func (mysqlRepository) CountUserMessagesAfter(sessionID, userID string, afterSeq int64) (int64, error) {
	return mysqlrepo.CountUserMessagesAfter(sessionID, userID, afterSeq)
}

func (mysqlRepository) CreateProfileConfirmation(c *models.ProfileConfirmation) error {
	return mysqlrepo.CreateProfileConfirmation(c)
}

func (mysqlRepository) ListPendingProfileConfirmations(userID string, limit int) ([]models.ProfileConfirmation, error) {
	return mysqlrepo.ListPendingProfileConfirmations(userID, limit)
}

func (mysqlRepository) FindProfileConfirmation(id uint, userID string) (*models.ProfileConfirmation, error) {
	return mysqlrepo.FindProfileConfirmation(id, userID)
}

func (mysqlRepository) MarkProfileConfirmation(id uint, userID, status string) (bool, error) {
	return mysqlrepo.MarkProfileConfirmation(id, userID, status)
}

func (mysqlRepository) CreateProfileEvent(e *models.ProfileEvent) error {
	return mysqlrepo.CreateProfileEvent(e)
}

var (
	repoMu   sync.RWMutex
	repoImpl repository = mysqlRepository{}
)

func currentRepository() repository {
	repoMu.RLock()
	defer repoMu.RUnlock()
	return repoImpl
}

// setRepositoryForTest 只在同包测试中使用，返回恢复函数。
func setRepositoryForTest(r repository) func() {
	repoMu.Lock()
	old := repoImpl
	repoImpl = r
	repoMu.Unlock()
	return func() {
		repoMu.Lock()
		repoImpl = old
		repoMu.Unlock()
	}
}
