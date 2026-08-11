package handlers

import (
	"ai-meeting/api/middleware"
	"ai-meeting/api/resp"
	"ai-meeting/dto"
	"ai-meeting/pkg/ecode"
	"ai-meeting/services/user"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type UserHandler struct {
	userService *user.UserService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		userService: user.GetUserService(),
	}
}

func (h *UserHandler) Login(c *gin.Context) {
	var req dto.UserLoginReqDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, ecode.RequestErr, err.Error())
		return
	}

	user, err := h.userService.Login(req)
	if err != nil {
		resp.Respond(c, err, nil)
		return
	}

	token, err := middleware.GenerateToken(user.Username, strconv.FormatUint(uint64(user.ID), 10))
	if err != nil {
		resp.Fail(c, ecode.ServerErr, "Failed to generate token")
		return
	}

	isAdmin, _ := h.userService.IsAdmin(user.Username)

	resp.Respond(c, nil, gin.H{
		"token":    token,
		"username": user.Username,
		"isAdmin":  isAdmin,
	})
}

func (h *UserHandler) Register(c *gin.Context) {
	var req dto.UserRegisterReqDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, ecode.RequestErr, err.Error())
		return
	}

	if err := h.userService.Register(req); err != nil {
		resp.Respond(c, err, nil)
		return
	}

	resp.Respond(c, nil, gin.H{"message": "Register success"})
}

func (h *UserHandler) GetUserByUsername(c *gin.Context) {
	username := c.Param("username")
	user, err := h.userService.GetUserByUsername(username)
	if err != nil {
		resp.Fail(c, ecode.NotExist, "User not found")
		return
	}

	resp.Respond(c, nil, dto.UserRespDTO{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Phone:    user.Phone,
		IsAdmin:  user.IsAdmin,
	})
}

func (h *UserHandler) HasUsername(c *gin.Context) {
	username := c.Query("username")
	exists, err := h.userService.HasUsername(username)
	if err != nil {
		resp.Respond(c, err, nil)
		return
	}

	resp.Respond(c, nil, exists)
}

func (h *UserHandler) Update(c *gin.Context) {
	var req dto.UserUpdateReqDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, ecode.RequestErr, err.Error())
		return
	}

	currentUsername, _ := c.Get("username")
	if currentUsername == nil {
		resp.Fail(c, ecode.NotLogin, "Unauthorized")
		return
	}

	if err := h.userService.Update(req, currentUsername.(string)); err != nil {
		resp.Respond(c, err, nil)
		return
	}

	resp.Respond(c, nil, gin.H{"message": "Update success"})
}

func (h *UserHandler) CheckLogin(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists || username == "" {
		resp.Respond(c, nil, gin.H{"isLogin": false})
		return
	}

	token := c.GetHeader("Authorization")
	resp.Respond(c, nil, gin.H{
		"isLogin":  true,
		"username": username,
		"token":    token,
	})
}

func (h *UserHandler) Logout(c *gin.Context) {
	resp.Respond(c, nil, gin.H{"message": "Logout success"})
}

func (h *UserHandler) IsAdmin(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists || username == "" {
		resp.Respond(c, nil, gin.H{"isAdmin": false})
		return
	}

	isAdmin, err := h.userService.IsAdmin(username.(string))
	if err != nil {
		logrus.Error(err)
		resp.Respond(c, nil, gin.H{"isAdmin": false})
		return
	}

	resp.Respond(c, nil, gin.H{
		"isAdmin":  isAdmin,
		"username": username,
	})
}

func (h *UserHandler) AddAdmin(c *gin.Context) {
	// 操作者必须是已登录的管理员
	operator, exists := c.Get("username")
	if !exists || operator == "" {
		resp.Fail(c, ecode.NotLogin, "Unauthorized")
		return
	}
	isAdmin, err := h.userService.IsAdmin(operator.(string))
	if err != nil {
		resp.Respond(c, err, nil)
		return
	}
	if !isAdmin {
		resp.Fail(c, ecode.NoPermission, "需要管理员权限")
		return
	}

	var username string
	if err := c.ShouldBindJSON(&username); err != nil {
		resp.Fail(c, ecode.RequestErr, err.Error())
		return
	}

	if err := h.userService.SetAdmin(username); err != nil {
		resp.Respond(c, err, nil)
		return
	}

	resp.Respond(c, nil, gin.H{"message": "Add admin success"})
}

func (h *UserHandler) PageUsers(c *gin.Context) {
	var req dto.UserPageReqDTO
	if err := c.ShouldBindQuery(&req); err != nil {
		resp.Fail(c, ecode.RequestErr, err.Error())
		return
	}

	users, total, err := h.userService.PageUsers(req)
	if err != nil {
		resp.Respond(c, err, nil)
		return
	}

	var respItems []dto.UserPageRespDTO
	for _, user := range users {
		respItems = append(respItems, dto.UserPageRespDTO{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Phone:     user.Phone,
			IsAdmin:   user.IsAdmin,
			Status:    user.Status,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	resp.Respond(c, nil, gin.H{
		"data":  respItems,
		"total": total,
	})
}
