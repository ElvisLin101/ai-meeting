package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	respx "ai-meeting/api/resp"
	"ai-meeting/dto"
	"ai-meeting/pkg/ecode"
	"ai-meeting/services/agent/context"
)

// ProfileController 画像确认项接口。
type ProfileController struct{}

// NewProfileController 创建画像控制器。
func NewProfileController() *ProfileController { return &ProfileController{} }

// ListConfirmations 查询当前用户待确认的画像项。
func (c *ProfileController) ListConfirmations(ctx *gin.Context) {
	user, ok := currentUsername(ctx)
	if !ok {
		respx.Fail(ctx, ecode.NotLogin, "Unauthorized")
		return
	}
	items, err := agentctx.ListPendingProfileConfirmations(ctx.Request.Context(), user)
	if err != nil {
		respx.Respond(ctx, ecode.Wrap(err, "查询画像确认项失败"), nil)
		return
	}
	out := make([]dto.ProfileConfirmationRespDTO, 0, len(items))
	for _, item := range items {
		out = append(out, dto.ProfileConfirmationRespDTO{
			ID: item.ID, Field: item.Field, Key: item.Key, Value: item.Value,
			Question: item.Question, Status: item.Status,
			CreatedAt: item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	respx.Respond(ctx, nil, out)
}

// ResolveConfirmation 确认或拒绝一条画像项。
func (c *ProfileController) ResolveConfirmation(ctx *gin.Context) {
	user, ok := currentUsername(ctx)
	if !ok {
		respx.Fail(ctx, ecode.NotLogin, "Unauthorized")
		return
	}
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respx.Fail(ctx, ecode.RequestErr, "无效的确认项 ID")
		return
	}
	var req dto.ResolveProfileConfirmationReqDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respx.Fail(ctx, ecode.RequestErr, err.Error())
		return
	}
	if err := agentctx.ResolveProfileConfirmation(ctx.Request.Context(), user, uint(id), req.Action); err != nil {
		respx.Respond(ctx, ecode.New(ecode.ErrAgentProfileInvalid, err.Error()), nil)
		return
	}
	status := "confirmed"
	if req.Action == "reject" {
		status = "rejected"
	}
	respx.Respond(ctx, nil, gin.H{"id": id, "status": status})
}

func currentUsername(ctx *gin.Context) (string, bool) {
	value, exists := ctx.Get("username")
	if !exists {
		return "", false
	}
	username, ok := value.(string)
	if !ok || username == "" {
		return "", false
	}
	return username, true
}
