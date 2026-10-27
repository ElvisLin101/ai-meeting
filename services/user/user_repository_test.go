package user

import (
	"errors"
	"strings"
	"testing"

	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/pkg/ecode"

	"gorm.io/gorm"
)

type fakeUserRepository struct {
	activeUser    *models.User
	activeErr     error
	findUser      *models.User
	findErr       error
	created       *models.User
	createErr     error
	count         int64
	countErr      error
	updateContact struct{ email, phone string }
	contactErr    error
	pageUsers     []models.User
	pageTotal     int64
	pageOffset    int
	pageLimit     int
	pageErr       error
	adminSet      string
	adminErr      error
	passwordUser  string
	passwordHash  string
	passwordErr   error
}

func (f *fakeUserRepository) FindActiveUserByUsername(string) (*models.User, error) {
	return f.activeUser, f.activeErr
}

func (f *fakeUserRepository) FindUserByUsername(string) (*models.User, error) {
	return f.findUser, f.findErr
}

func (f *fakeUserRepository) CreateUser(user *models.User) error {
	f.created = user
	return f.createErr
}

func (f *fakeUserRepository) CountUsersByUsername(string) (int64, error) {
	return f.count, f.countErr
}

func (f *fakeUserRepository) UpdateUserContact(_ string, email, phone string) error {
	f.updateContact.email, f.updateContact.phone = email, phone
	return f.contactErr
}

func (f *fakeUserRepository) PageUsers(_ string, offset, limit int) ([]models.User, int64, error) {
	f.pageOffset, f.pageLimit = offset, limit
	return f.pageUsers, f.pageTotal, f.pageErr
}

func (f *fakeUserRepository) SetUserAdmin(username string) error {
	f.adminSet = username
	return f.adminErr
}

func (f *fakeUserRepository) UpdateUserPassword(username, passwordHash string) error {
	f.passwordUser, f.passwordHash = username, passwordHash
	return f.passwordErr
}

func TestUserServiceLoginSuccess(t *testing.T) {
	hash, _ := hashPassword("secret")
	repo := &fakeUserRepository{activeUser: &models.User{Username: "alice", Password: hash}}
	svc := &UserService{repo: repo}

	user, err := svc.Login(dto.UserLoginReqDTO{Username: "alice", Password: "secret"})
	if err != nil || user == nil || user.Username != "alice" {
		t.Fatalf("login failed: user=%+v err=%v", user, err)
	}
	if repo.passwordHash != "" {
		t.Fatal("bcrypt password should not be migrated")
	}
}

func TestUserServiceLoginNotFound(t *testing.T) {
	repo := &fakeUserRepository{activeErr: gorm.ErrRecordNotFound}
	svc := &UserService{repo: repo}

	_, err := svc.Login(dto.UserLoginReqDTO{Username: "missing", Password: "x"})
	if err == nil || ecode.Cause(err).Code() != ecode.ErrUserNotFound {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestUserServiceLoginWrongPassword(t *testing.T) {
	hash, _ := hashPassword("secret")
	repo := &fakeUserRepository{activeUser: &models.User{Username: "alice", Password: hash}}
	svc := &UserService{repo: repo}

	_, err := svc.Login(dto.UserLoginReqDTO{Username: "alice", Password: "wrong"})
	if err == nil || ecode.Cause(err).Code() != ecode.ErrWrongPassword {
		t.Fatalf("want ErrWrongPassword, got %v", err)
	}
}

func TestUserServiceLoginMigratesPlaintextPassword(t *testing.T) {
	repo := &fakeUserRepository{activeUser: &models.User{Username: "legacy", Password: "plain"}}
	svc := &UserService{repo: repo}

	if _, err := svc.Login(dto.UserLoginReqDTO{Username: "legacy", Password: "plain"}); err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if repo.passwordUser != "legacy" || !strings.HasPrefix(repo.passwordHash, "$2") {
		t.Fatalf("plaintext password should be migrated, got user=%q hash=%q", repo.passwordUser, repo.passwordHash)
	}
}

func TestUserServiceRegisterExisting(t *testing.T) {
	repo := &fakeUserRepository{findUser: &models.User{Username: "alice"}}
	svc := &UserService{repo: repo}

	err := svc.Register(dto.UserRegisterReqDTO{Username: "alice", Password: "secret"})
	if err == nil || ecode.Cause(err).Code() != ecode.ErrUsernameExists {
		t.Fatalf("want ErrUsernameExists, got %v", err)
	}
	if repo.created != nil {
		t.Fatal("existing user should not be created")
	}
}

func TestUserServiceRegisterCreatesHashedUser(t *testing.T) {
	repo := &fakeUserRepository{findErr: gorm.ErrRecordNotFound}
	svc := &UserService{repo: repo}

	err := svc.Register(dto.UserRegisterReqDTO{
		Username: "alice", Password: "secret", Email: "a@example.com", Phone: "123",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if repo.created == nil || repo.created.Username != "alice" || repo.created.Email != "a@example.com" {
		t.Fatalf("unexpected created user: %+v", repo.created)
	}
	if !strings.HasPrefix(repo.created.Password, "$2") || repo.created.Password == "secret" {
		t.Fatalf("password should be bcrypt hashed, got %q", repo.created.Password)
	}
}

func TestUserServicePageUsersNormalizesPagination(t *testing.T) {
	repo := &fakeUserRepository{}
	svc := &UserService{repo: repo}

	_, _, err := svc.PageUsers(dto.UserPageReqDTO{Page: 0, Size: 1000})
	if err != nil {
		t.Fatalf("page users: %v", err)
	}
	if repo.pageOffset != 0 || repo.pageLimit != 100 {
		t.Fatalf("unexpected pagination offset=%d limit=%d", repo.pageOffset, repo.pageLimit)
	}
}

func TestUserServiceIsAdmin(t *testing.T) {
	repo := &fakeUserRepository{findUser: &models.User{Username: "admin", IsAdmin: true}}
	svc := &UserService{repo: repo}

	ok, err := svc.IsAdmin("admin")
	if err != nil || !ok {
		t.Fatalf("IsAdmin = %v, %v", ok, err)
	}

	repo.findErr = errors.New("db down")
	if _, err := svc.IsAdmin("admin"); err == nil {
		t.Fatal("repo error should surface")
	}
}
