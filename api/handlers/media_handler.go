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
