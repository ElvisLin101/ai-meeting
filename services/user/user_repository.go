package user

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
)

// userRepository 是 UserService 需要的最小用户存储接口。
// 生产实现走 MySQL；测试可注入内存 fake，不需要 sqlmock 或全局 DB。
type userRepository interface {
	FindActiveUserByUsername(username string) (*models.User, error)
	FindUserByUsername(username string) (*models.User, error)
	CreateUser(user *models.User) error
	CountUsersByUsername(username string) (int64, error)
	UpdateUserContact(currentUsername, email, phone string) error
	PageUsers(username string, offset, limit int) ([]models.User, int64, error)
	SetUserAdmin(username string) error
	UpdateUserPassword(username, passwordHash string) error
}

type mysqlUserRepository struct{}

func (mysqlUserRepository) FindActiveUserByUsername(username string) (*models.User, error) {
	return mysqlrepo.FindActiveUserByUsername(username)
}

func (mysqlUserRepository) FindUserByUsername(username string) (*models.User, error) {
	return mysqlrepo.FindUserByUsername(username)
}

func (mysqlUserRepository) CreateUser(user *models.User) error {
	return mysqlrepo.CreateUser(user)
}

func (mysqlUserRepository) CountUsersByUsername(username string) (int64, error) {
	return mysqlrepo.CountUsersByUsername(username)
}

func (mysqlUserRepository) UpdateUserContact(currentUsername, email, phone string) error {
	return mysqlrepo.UpdateUserContact(currentUsername, email, phone)
}

func (mysqlUserRepository) PageUsers(username string, offset, limit int) ([]models.User, int64, error) {
	return mysqlrepo.PageUsers(username, offset, limit)
}

func (mysqlUserRepository) SetUserAdmin(username string) error {
	return mysqlrepo.SetUserAdmin(username)
}

func (mysqlUserRepository) UpdateUserPassword(username, passwordHash string) error {
	return mysqlrepo.UpdateUserPassword(username, passwordHash)
}
