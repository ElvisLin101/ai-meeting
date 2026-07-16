package handlers

import (
	"ai-meeting/api/middleware"
	"ai-meeting/dto"
	"ai-meeting/services/user"
	"net/http"

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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.Login(req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	token, err := middleware.GenerateToken(user.Username, string(rune(user.ID)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	isAdmin, _ := h.userService.IsAdmin(user.Username)

	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"username": user.Username,
		"isAdmin":  isAdmin,
	})
}

func (h *UserHandler) Register(c *gin.Context) {
	var req dto.UserRegisterReqDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userService.Register(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Register success"})
}

func (h *UserHandler) GetUserByUsername(c *gin.Context) {
	username := c.Param("username")
	user, err := h.userService.GetUserByUsername(username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, dto.UserRespDTO{
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, exists)
}

func (h *UserHandler) Update(c *gin.Context) {
	var req dto.UserUpdateReqDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	currentUsername, _ := c.Get("username")
	if currentUsername == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := h.userService.Update(req, currentUsername.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Update success"})
}

func (h *UserHandler) CheckLogin(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists || username == "" {
		c.JSON(http.StatusOK, gin.H{"isLogin": false})
		return
	}

	token := c.GetHeader("Authorization")
	c.JSON(http.StatusOK, gin.H{
		"isLogin":  true,
		"username": username,
		"token":    token,
	})
}

func (h *UserHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Logout success"})
}

func (h *UserHandler) IsAdmin(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists || username == "" {
		c.JSON(http.StatusOK, gin.H{"isAdmin": false})
		return
	}

	isAdmin, err := h.userService.IsAdmin(username.(string))
	if err != nil {
		logrus.Error(err)
		c.JSON(http.StatusOK, gin.H{"isAdmin": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"isAdmin":  isAdmin,
		"username": username,
	})
}

func (h *UserHandler) AddAdmin(c *gin.Context) {
	var username string
	if err := c.ShouldBindJSON(&username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userService.SetAdmin(username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Add admin success"})
}

func (h *UserHandler) PageUsers(c *gin.Context) {
	var req dto.UserPageReqDTO
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	users, total, err := h.userService.PageUsers(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []dto.UserPageRespDTO
	for _, user := range users {
		resp = append(resp, dto.UserPageRespDTO{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Phone:     user.Phone,
			IsAdmin:   user.IsAdmin,
			Status:    user.Status,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  resp,
		"total": total,
	})
}
