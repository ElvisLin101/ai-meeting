package user

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/pkg/ecode"
	mysqlrepo "ai-meeting/repositories/mysql"
	"errors"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserService struct{}

// hashPassword bcrypt 哈希密码
func hashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyPassword 校验密码。
// 返回 (是否匹配, 是否旧明文需迁移, 错误)。
// 旧明文存储(无 bcrypt 前缀)直接比较并标记迁移——历史明文数据登录成功后回写哈希, 平滑升级。
func verifyPassword(stored, plain string) (matched, needMigrate bool, err error) {
	if !strings.HasPrefix(stored, "$2") {
		return stored == plain, true, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain)); err != nil {
		return false, false, nil
	}
	return true, false, nil
}

// Login 用户登录验证
func (s *UserService) Login(req dto.UserLoginReqDTO) (*models.User, error) {
	user, err := mysqlrepo.FindActiveUserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ecode.New(ecode.ErrUserNotFound, "user not found")
		}
		return nil, err
	}
	matched, migrate, err := verifyPassword(user.Password, req.Password)
	if err != nil {
		return nil, ecode.Wrap(err, "密码校验失败")
	}
	if !matched {
		return nil, ecode.New(ecode.ErrWrongPassword, "invalid password")
	}
	// 旧明文用户登录成功后自动迁移为 bcrypt 哈希
	if migrate {
		if hashed, herr := hashPassword(req.Password); herr == nil {
			if uerr := mysqlrepo.UpdateUserPassword(user.Username, hashed); uerr != nil {
				logrus.Warnf("Failed to migrate password hash for user %s: %v", user.Username, uerr)
			}
		}
	}
	return user, nil
}

// Register 用户注册
func (s *UserService) Register(req dto.UserRegisterReqDTO) error {
	if _, err := mysqlrepo.FindUserByUsername(req.Username); err == nil {
		return ecode.New(ecode.ErrUsernameExists, "username already exists")
	}
	hashed, err := hashPassword(req.Password)
	if err != nil {
		return ecode.Wrap(err, "密码哈希失败")
	}
	user := models.User{Username: req.Username, Password: hashed, Email: req.Email, Phone: req.Phone, IsAdmin: false, Status: 1}
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
	// 归一化分页参数, 防止 Page=0 产生负 offset / Size 超大拖垮查询
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Size < 1 {
		req.Size = 10
	}
	if req.Size > 100 {
		req.Size = 100
	}
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
