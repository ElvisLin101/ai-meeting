package handlers

import (
	respx "ai-meeting/api/resp"
	"ai-meeting/dto"
	"ai-meeting/pkg/ecode"
	mysqlrepo "ai-meeting/repositories/mysql"
	"ai-meeting/services/agent/loop"
	agentrun "ai-meeting/services/agent/run"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AgentRunController Agent run 入口（Agent 模块主入口）
type AgentRunController struct {
	service *agentrun.Service
}

// NewAgentRunController 创建控制器
func NewAgentRunController() *AgentRunController {
	return &AgentRunController{service: agentrun.GetService()}
}

// Run 非流式执行一次 Agent run
func (c *AgentRunController) Run(ctx *gin.Context) {
	req, ok := c.bindRequest(ctx)
	if !ok {
		return
	}
	result, err := c.service.Start(ctx.Request.Context(), req)
	if err != nil {
		respx.Respond(ctx, err, nil)
		return
	}
	respx.Respond(ctx, nil, result)
}

// RunStream SSE 流式执行
func (c *AgentRunController) RunStream(ctx *gin.Context) {
	req, ok := c.bindRequest(ctx)
	if !ok {
		return
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)
	ctx.Writer.Flush()

	emit := func(ev loop.Event) {
		if ctx.Request.Context().Err() != nil {
			return
		}
		// 最终 done 由 handler 统一发送（带 run_id）
		if ev.Type == loop.EventDone {
			return
		}
		payload, _ := json.Marshal(ev)
		ctx.SSEvent(string(ev.Type), string(payload))
		ctx.Writer.Flush()
	}

	result, err := c.service.StartStream(ctx.Request.Context(), req, emit)
	if err != nil {
		ctx.SSEvent("error", err.Error())
		ctx.Writer.Flush()
		return
	}
	payload, _ := json.Marshal(result)
	ctx.SSEvent("done", string(payload))
	ctx.Writer.Flush()
}

// Get 查询 run 详情（含 step 回放）
func (c *AgentRunController) Get(ctx *gin.Context) {
	username, _ := ctx.Get("username")
	runID := ctx.Param("runId")

	record, err := mysqlrepo.FindAgentRun(runID)
	if err != nil {
		respx.Respond(ctx, ecode.Wrap(err, "查询 run 失败"), nil)
		return
	}
	if record == nil || record.UserID != username {
		respx.Fail(ctx, ecode.ErrAgentRunNotFound, "run 不存在或无权限")
		return
	}
	steps, err := mysqlrepo.ListAgentSteps(runID)
	if err != nil {
		respx.Respond(ctx, ecode.Wrap(err, "查询步骤失败"), nil)
		return
	}

	detail := dto.AgentRunDetailRespDTO{
		RunID: record.RunID, SessionID: record.SessionID, Status: record.Status,
		StopReason: record.StopReason, Input: record.Input, Output: record.Output,
		Steps: record.Steps, Tokens: record.Tokens, Error: record.Error,
		PlanJSON:          record.PlanJSON,
		ContextReportJSON: record.ContextReportJSON,
		StartedAt:         record.StartedAt.Format("2006-01-02 15:04:05"),
	}
	if record.FinishedAt != nil {
		detail.FinishedAt = record.FinishedAt.Format("2006-01-02 15:04:05")
	}
	for _, s := range steps {
		detail.StepDetail = append(detail.StepDetail, dto.AgentRunStepRespDTO{
			StepIndex: s.StepIndex, Thought: s.Thought, Action: s.Action,
			ActionInput: s.ActionInput, Observation: s.Observation,
			Status: s.Status, DurationMs: s.DurationMs, Error: s.Error,
		})
	}
	respx.Respond(ctx, nil, detail)
}

// Page 分页查询当前用户的 run
func (c *AgentRunController) Page(ctx *gin.Context) {
	username, _ := ctx.Get("username")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	runs, total, err := mysqlrepo.PageAgentRuns(username.(string), (page-1)*size, size)
	if err != nil {
		respx.Respond(ctx, ecode.Wrap(err, "查询 run 列表失败"), nil)
		return
	}
	items := make([]dto.AgentRunRespDTO, 0, len(runs))
	for _, r := range runs {
		items = append(items, dto.AgentRunRespDTO{
			RunID: r.RunID, SessionID: r.SessionID, Status: r.Status,
			StopReason: r.StopReason, Output: r.Output, Steps: r.Steps, Tokens: r.Tokens,
		})
	}
	ctx.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

// bindRequest 绑定请求并读取认证上下文
func (c *AgentRunController) bindRequest(ctx *gin.Context) (agentrun.StartRequest, bool) {
	var body dto.AgentRunReqDTO
	if err := ctx.ShouldBindJSON(&body); err != nil {
		respx.Fail(ctx, ecode.RequestErr, err.Error())
		return agentrun.StartRequest{}, false
	}
	username, ok := ctx.Get("username")
	if !ok || username == "" {
		respx.Fail(ctx, ecode.NotLogin, "Unauthorized")
		return agentrun.StartRequest{}, false
	}
	userID, ok := ctx.Get("user_id")
	if !ok || userID == "" {
		respx.Fail(ctx, ecode.NotLogin, "Unauthorized")
		return agentrun.StartRequest{}, false
	}
	return agentrun.StartRequest{
		UserID:    userID.(string),
		Username:  username.(string),
		SessionID: body.SessionID,
		AgentID:   body.AgentID,
		Input:     body.Input,
	}, true
}
