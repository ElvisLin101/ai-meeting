package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type XunfeiTtsController struct{}

func NewXunfeiTtsController() *XunfeiTtsController {
	return &XunfeiTtsController{}
}

func (c *XunfeiTtsController) CreateTask(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"task_id": "mock-task-id", "status": "pending"})
}

func (c *XunfeiTtsController) QueryTask(ctx *gin.Context) {
	taskId := ctx.Param("taskId")
	ctx.JSON(http.StatusOK, gin.H{"task_id": taskId, "status": "completed"})
}

func (c *XunfeiTtsController) SynthesizeAndWait(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"task_id": "mock-task-id", "status": "completed"})
}

type WebSocketController struct{}

func NewWebSocketController() *WebSocketController {
	return &WebSocketController{}
}

func (c *WebSocketController) CheckUserStatus(ctx *gin.Context) {
	userId := ctx.Param("userId")
	ctx.JSON(http.StatusOK, gin.H{"userId": userId, "isOnline": false})
}

func (c *WebSocketController) SendMessage(ctx *gin.Context) {
	userId := ctx.Query("userId")
	messageType := ctx.Query("type")
	data := ctx.Query("data")

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message delivered",
		"userId":  userId,
		"type":    messageType,
		"content": ctx.Query("message"),
		"data":    data,
	})
}

func (c *WebSocketController) SendNotification(ctx *gin.Context) {
	userId := ctx.Param("userId")

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Notification delivered",
		"userId":  userId,
	})
}

func (c *WebSocketController) SendTranscriptionResult(ctx *gin.Context) {
	userId := ctx.Param("userId")
	result := ctx.Query("result")
	isFinal := ctx.DefaultQuery("isFinal", "false")

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Transcription delivered",
		"userId":  userId,
		"result":  result,
		"isFinal": isFinal,
	})
}

func (c *WebSocketController) SendErrorMessage(ctx *gin.Context) {
	userId := ctx.Param("userId")
	errorMessage := ctx.Query("errorMessage")

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Error message delivered",
		"userId":  userId,
		"error":   errorMessage,
	})
}
