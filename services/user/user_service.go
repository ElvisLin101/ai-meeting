package user

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"errors"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type UserService struct{}

// Login 用户登录验证
func (s *UserService) Login(req dto.UserLoginReqDTO) (*models.User, error) {
	user, err := mysqlrepo.FindActiveUserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	if user.Password != req.Password {
		return nil, errors.New("invalid password")
	}
	return user, nil
}

// Register 用户注册
func (s *UserService) Register(req dto.UserRegisterReqDTO) error {
	if _, err := mysqlrepo.FindUserByUsername(req.Username); err == nil {
		return errors.New("username already exists")
	}
	user := models.User{Username: req.Username, Password: req.Password, Email: req.Email, Phone: req.Phone, IsAdmin: false, Status: 1}
	return mysqlrepo.CreateUser(&user)
}

// GetUserByUsername 根据用户名查询用户
func (s *UserService) GetUserByUsername(username string) (*models.User, error) {
	return mysqlrepo.FindUserByUsername(username)
}

// HasUsername 检查用户名是否存在
func (s *UserService) HasUsername(username string) (bool, error) {
	count, err := mysqlrepo.CountUsersByUsername(username)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Update 更新用户信息
func (s *UserService) Update(req dto.UserUpdateReqDTO, currentUsername string) error {
	return mysqlrepo.UpdateUserContact(currentUsername, req.Email, req.Phone)
}

// PageUsers 分页查询用户列表
func (s *UserService) PageUsers(req dto.UserPageReqDTO) ([]models.User, int64, error) {
	offset := (req.Page - 1) * req.Size
	return mysqlrepo.PageUsers(req.Username, offset, req.Size)
}

// IsAdmin 检查用户是否为管理员
func (s *UserService) IsAdmin(username string) (bool, error) {
	user, err := mysqlrepo.FindUserByUsername(username)
	if err != nil {
		return false, err
	}
	return user.IsAdmin, nil
}

// SetAdmin 设置用户为管理员
func (s *UserService) SetAdmin(username string) error {
	return mysqlrepo.SetUserAdmin(username)
}

var userServiceInstance *UserService

// GetUserService 获取UserService单例
func GetUserService() *UserService {
	if userServiceInstance == nil {
		userServiceInstance = &UserService{}
	}
	logrus.Info("UserService instance created")
	return userServiceInstance
}
