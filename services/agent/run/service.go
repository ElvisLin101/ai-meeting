package run

import (
	"ai-meeting/clients"
	"ai-meeting/config"
	"ai-meeting/models"
	"ai-meeting/pkg/ecode"
	"ai-meeting/pkg/singleflight"
	"ai-meeting/repositories"
	"ai-meeting/services/agent/context"
	"ai-meeting/services/agent/loop"
	planpkg "ai-meeting/services/agent/plan"
	"ai-meeting/services/agent/tools"
	"ai-meeting/services/metric"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ============================================================
// Agent Run 编排服务
//
// 职责: 会话归属 → 消息持久化(MySQL 真相源 + Redis 热缓存) → 四层上下文
//       → Agent 循环 → run/step 落库 → 异步压缩与画像提炼。
// ============================================================

// StartRequest 发起一次 run
type StartRequest struct {
	UserID    string // JWT user_id（面试工具使用）
	Username  string // JWT username（Agent 会话归属）
	SessionID string // 可空, 空则新建 Agent 会话
	AgentID   uint
	Input     string
}

// StartResult run 结果
type StartResult struct {
	RunID      string `json:"run_id"`
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	StopReason string `json:"stop_reason"`
	Output     string `json:"output"`
	Steps      int    `json:"steps"`
	Tokens     int    `json:"tokens"`
	PlanMode   string `json:"plan_mode,omitempty"` // plan / plan_fallback / direct
}

// Service Agent run 服务
type Service struct {
	repo      runRepository
	registry  *tools.Registry
	executor  *tools.Executor
	assembler *agentctx.Assembler
	cfg       config.AgentConfig
	planCfg   planpkg.Config
	quota     config.QuotaConfig
}

func (s *Service) repository() runRepository {
	if s.repo != nil {
		return s.repo
	}
	return mysqlRunRepository{}
}

var (
	serviceInstance *Service
	serviceOnce     sync.Once
)

// GetService 获取单例
func GetService() *Service {
	serviceOnce.Do(func() {
		cfg := config.GetConfig().Agent
		registry := buildRegistry(cfg)
		executor := tools.NewExecutor(registry,
			time.Duration(cfg.ToolTimeoutSec)*time.Second, cfg.MaxToolResultBytes)
		executor.SetMetricFunc(func(tool string, success bool, durationMs int64, errType string) {
			metric.GetMetricService().Record(models.MetricLog{
				Module: "agent_tool", Event: tool, Success: success,
				DurationMs: durationMs, ErrorType: errType,
			})
		})
		serviceInstance = &Service{
			repo:     mysqlRunRepository{},
			registry: registry,
			executor: executor,
			assembler: agentctx.NewAssembler(agentctx.Config{
				PromptBudgetTokens:      contextPromptBudget(cfg.Context),
				IdentityMaxTokens:       cfg.Context.IdentityMaxTokens,
				ProfileMaxTokens:        cfg.Context.ProfileMaxTokens,
				SummaryMaxTokens:        cfg.Context.SummaryMaxTokens,
				RecentMaxTokens:         cfg.Context.RecentMaxTokens,
				CompressThresholdTokens: cfg.Context.CompressThresholdTokens,
			}),
			cfg: cfg,
			planCfg: planpkg.Config{
				Enabled:          cfg.Plan.Enabled,
				MaxTasks:         cfg.Plan.MaxTasks,
				MaxParallel:      cfg.Plan.MaxParallel,
				TaskTimeout:      time.Duration(cfg.Plan.TaskTimeoutSec) * time.Second,
				TotalTimeout:     time.Duration(cfg.Plan.TotalTimeoutSec) * time.Second,
				MaxStepsPerTask:  cfg.Plan.MaxStepsPerTask,
				LongQueryRunes:   cfg.Plan.LongQueryRunes,
				ReplanEnabled:    cfg.Plan.ReplanEnabled,
				MaxReplans:       cfg.Plan.MaxReplans,
				RetryMaxAttempts: cfg.Plan.RetryMaxAttempts,
				RetryBaseBackoff: time.Duration(cfg.Plan.RetryBackoffMs) * time.Millisecond,
				RouteMode:        cfg.Plan.RouteMode,
				RouteMinScore:    cfg.Plan.RouteMinScore,
				FastPathRunes:    cfg.Plan.FastPathRunes,
			},
			quota: cfg.Quota,
		}
	})
	return serviceInstance
}

// buildRegistry 装配内置工具
func buildRegistry(cfg config.AgentConfig) *tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewPDFTool(cfg.SandboxRoot))
	registry.Register(tools.NewFileListTool(cfg.SandboxRoot))
	registry.Register(tools.NewFileReadTool(cfg.SandboxRoot))
	registry.Register(tools.NewFileWriteTool(cfg.SandboxRoot))
	registry.Register(tools.NewFileEditTool(cfg.SandboxRoot))
	registry.Register(tools.NewInterviewStartTool())
	registry.Register(tools.NewInterviewCurrentQuestionTool())
	registry.Register(tools.NewInterviewSubmitAnswerTool())

	searchCfg := cfg.Search
	providerName := strings.ToLower(strings.TrimSpace(searchCfg.Provider))
	if providerName == "" {
		providerName = "deepseek"
	}
	// DeepSeek 的 Search/Fetch 复用模型 API key，不要求用户再配一份搜索 key。
	if providerName == "deepseek" && strings.TrimSpace(searchCfg.APIKey) == "" {
		searchCfg.APIKey = config.GetConfig().AI.DeepSeek.APIKey
	}
	if provider, err := tools.NewSearchProvider(searchCfg); err != nil {
		logrus.Warnf("[agent] web_search disabled: %v", err)
	} else {
		registry.Register(tools.NewWebSearchTool(provider, searchCfg.MaxResults))
		if fetcher, ok := provider.(tools.PageFetcher); ok {
			registry.Register(tools.NewWebFetchTool(fetcher))
		} else {
			logrus.Infof("[agent] web_fetch disabled: provider %s does not implement fetch", providerName)
		}
	}
	return registry
}

// Start 非流式执行（同一用户+会话+输入的并发请求通过 SingleFlight 合并）
func (s *Service) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	if err := validate(req); err != nil {
		return nil, err
	}
	key := dedupKey(req)
	result, err := repositories.SingleFlight.Do(ctx, key, func(ctx context.Context, _ *singleflight.StreamWriter) (interface{}, error) {
		return s.runWithQuota(ctx, req.Username, func() (*StartResult, error) {
			return s.execute(ctx, req, nil)
		})
	})
	if err != nil {
		return nil, err
	}
	return decodeStartResult(result)
}

// decodeStartResult 兼容 SingleFlight 的两种返回：
//   - leader 返回本进程的 *StartResult；
//   - follower 从 Redis 读回 JSON 后得到 map[string]interface{}。
func decodeStartResult(result interface{}) (*StartResult, error) {
	switch v := result.(type) {
	case *StartResult:
		if v == nil {
			return nil, fmt.Errorf("empty singleflight result")
		}
		return v, nil
	case StartResult:
		return &v, nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("encode singleflight result: %w", err)
		}
		var out StartResult
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode singleflight result: %w", err)
		}
		return &out, nil
	default:
		return nil, fmt.Errorf("unexpected singleflight result type %T", result)
	}
}

// StartStream 流式执行（用 Redis 幂等锁防止重复提交；SingleFlight 不复用流式 chunk）
func (s *Service) StartStream(ctx context.Context, req StartRequest, emit func(loop.Event)) (*StartResult, error) {
	if err := validate(req); err != nil {
		return nil, err
	}
	release, err := acquireRunLock(ctx, dedupKey(req))
	if err != nil {
		return nil, err
	}
	defer release()
	return s.runWithQuota(ctx, req.Username, func() (*StartResult, error) {
		return s.execute(ctx, req, emit)
	})
}

// execute 核心执行流程
func (s *Service) execute(ctx context.Context, req StartRequest, emit func(loop.Event)) (*StartResult, error) {
	conv, err := s.ensureConversation(req)
	if err != nil {
		return nil, err
	}

	// 1. 保存用户消息（MySQL 先写, Redis 后写）
	userMsg, err := s.saveMessage(conv.SessionID, req.Username, "user", req.Input, "")
	if err != nil {
		return nil, ecode.Wrap(err, "保存用户消息失败")
	}
	// L1 长期记忆候选：只做规则筛选并落库，不调用 LLM、不直接改画像。
	agentctx.CaptureCandidatesAsync(req.Username, conv.SessionID, userMsg.ID, req.Input)

	// 2. 四层上下文
	buildAssembled := func() (*agentctx.Assembled, error) {
		return s.assembler.Build(ctx, agentctx.BuildInput{
			UserID: req.Username, SessionID: conv.SessionID, AgentID: req.AgentID,
			Query: req.Input,
		})
	}
	assembled, err := buildAssembled()
	if err != nil {
		return nil, ecode.Wrap(err, "加载上下文失败")
	}
	if assembled.DroppedMessages > 0 {
		// 装配时已经不得不丢近期消息：先同步压缩一次，再重新装配。
		// 这样不会把“未被摘要覆盖”的旧消息静默丢掉。
		logrus.Warnf("[agent-run] context budget dropped %d recent messages, running preflight compression (session=%s covered=%d)",
			assembled.DroppedMessages, conv.SessionID, assembled.CoveredThroughSeq)
		compressCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		compressErr := agentctx.CompressIfNeeded(compressCtx, conv.SessionID, req.Username, s.cfg.Context.CompressThresholdTokens)
		cancel()
		if compressErr != nil {
			logrus.Warnf("[agent-run] preflight compression failed: %v", compressErr)
		} else if rebuilt, rebuildErr := buildAssembled(); rebuildErr == nil {
			assembled = rebuilt
		} else {
			logrus.Warnf("[agent-run] rebuild context after compression failed: %v", rebuildErr)
		}
		if assembled.DroppedMessages > 0 {
			logrus.Warnf("[agent-run] context still dropped %d recent messages after preflight compression (session=%s)",
				assembled.DroppedMessages, conv.SessionID)
		}
	}
	initial := make([]clients.ToolMessage, 0, len(assembled.Messages))
	for _, m := range assembled.Messages {
		initial = append(initial, clients.ToolMessage{Role: m.Role, Content: m.Content})
	}
	// 去掉刚保存的当前用户消息（循环里会作为 userInput 追加）
	if n := len(initial); n > 0 && initial[n-1].Role == "user" && initial[n-1].Content == req.Input {
		initial = initial[:n-1]
	}

	// 3. 创建 run
	runID := uuid.NewString()
	runRecord := &models.AgentRun{
		RunID: runID, SessionID: conv.SessionID, UserID: req.Username,
		AgentID: req.AgentID, Input: req.Input, Status: models.AgentRunStatusRunning,
		StartedAt: time.Now(),
	}
	if err := s.repository().CreateAgentRun(runRecord); err != nil {
		return nil, ecode.Wrap(err, "创建 run 失败")
	}
	if raw, err := json.Marshal(assembled.Report); err == nil {
		if err := s.repository().UpdateAgentRunContextReport(runID, string(raw)); err != nil {
			logrus.Warnf("[agent-run] persist context report failed: %v", err)
		}
	}
	logrus.Infof("[agent-run] context report: mode=%s used=%d/%d identity=%d profile=%d summary=%d recent=%d dropped=%d",
		assembled.Report.Mode, assembled.Report.UsedTokens, assembled.Report.BudgetTokens,
		assembled.Report.IdentityTokens, assembled.Report.ProfileTokens,
		assembled.Report.SummaryTokens, assembled.Report.RecentTokens, assembled.Report.DroppedMessages)

	tc := &tools.Context{
		UserID: req.UserID, Username: req.Username,
		SessionID: conv.SessionID, RunID: runID, AgentID: req.AgentID,
	}

	// Plan-and-Execute：先做轻量路由，简单请求直接走线性 ReAct；
	// 复杂请求才调用 planner。规划失败仍回退线性 ReAct，不阻断用户请求。
	planStart := time.Now()
	planSvc := planpkg.NewService(s.planCfg, s.registry, s.executor)
	decision := planSvc.Decide(req.Input)
	if emit != nil {
		emit(loop.Event{
			Type:       loop.EventRoute,
			Content:    decision.Mode,
			StopReason: decision.Reason,
		})
	}
	planMode := "direct"
	if decision.UsePlan {
		planMode = "plan_fallback"
	}

	if decision.UsePlan {
		logrus.Infof("[agent-run] route=plan reason=%s", decision.Reason)
		// plan 的多个子任务共享同一个 run_id；step_index 在同一 run 内必须唯一，
		// 因此用 run 级原子序号串行分配，避免并行任务或重试写入相同 (run_id, step_index)。
		var planStepSeq atomic.Int64
		planResult, planErr := planSvc.Run(ctx, planpkg.Input{
			Query:       req.Input,
			ContextHint: buildContextHint(assembled),
			UserID:      req.UserID,
			Username:    req.Username,
			SessionID:   conv.SessionID,
			RunID:       runID,
			AgentID:     req.AgentID,
			RecorderFor: func(taskID string) loop.Recorder {
				return &stepRecorder{repo: s.repo, runID: runID, taskID: taskID, seq: &planStepSeq}
			},
			PlanSink: func(p *planpkg.Plan) {
				raw, err := json.Marshal(p)
				if err != nil {
					return
				}
				if err := s.repository().UpdateAgentRunPlan(runID, string(raw)); err != nil {
					logrus.Warnf("[agent-run] persist plan snapshot failed: %v", err)
				}
			},
		}, emit)
		if planErr != nil {
			logrus.Warnf("[agent-run] plan failed, fallback to react: %v", planErr)
		}
		if planResult != nil {
			return s.finishPlanRun(req, conv, runID, planResult, planStart), nil
		}
	} else {
		logrus.Debugf("[agent-run] route=direct reason=%s", decision.Reason)
	}

	recorder := &stepRecorder{repo: s.repo, runID: runID}
	runner := loop.New(s.registry, s.executor, loop.Config{
		AIID:                       req.AgentID,
		MaxSteps:                   s.cfg.MaxSteps,
		TotalTimeout:               time.Duration(s.cfg.TotalTimeoutSec) * time.Second,
		MaxRepeatActions:           3,
		MaxConsecutiveToolFailures: 3,
		MaxParallelToolCalls:       s.cfg.MaxParallelTools,
		Temperature:                0.7,
	}, recorder)

	start := time.Now()
	loopResult, loopErr := runner.Run(ctx, tc, initial, req.Input, emit)
	durationMs := time.Since(start).Milliseconds()

	if loopErr != nil {
		_ = s.repository().FinishAgentRun(runID, models.AgentRunStatusFailed, "error", "", loopErr.Error(), 0, 0)
		metric.GetMetricService().Record(models.MetricLog{
			Module: "agent", Event: "run_failed", Success: false,
			SessionID: conv.SessionID, DurationMs: durationMs,
		})
		return nil, loopErr
	}

	// 4. 保存 assistant 消息
	if _, err := s.saveMessage(conv.SessionID, req.Username, "assistant", loopResult.Content, ""); err != nil {
		logrus.Warnf("[agent-run] save assistant message failed: %v", err)
	}

	status := models.AgentRunStatusCompleted
	if loopResult.StopReason != "final_answer" {
		status = models.AgentRunStatusMaxSteps
	}
	_ = s.repository().FinishAgentRun(runID, status, loopResult.StopReason,
		loopResult.Content, "", loopResult.Steps, loopResult.Tokens)

	metric.GetMetricService().Record(models.MetricLog{
		Module: "agent", Event: "run_completed", Success: true,
		SessionID: conv.SessionID, DurationMs: durationMs,
		Extra: fmt.Sprintf(`{"steps":%d,"tokens":%d,"stop_reason":%q}`,
			loopResult.Steps, loopResult.Tokens, loopResult.StopReason),
	})

	// 5. 异步压缩 + 画像 L2 consolidation
	agentctx.CompressAsync(conv.SessionID, req.Username, s.cfg.Context.CompressThresholdTokens)
	agentctx.ConsolidateAsync(req.Username, conv.SessionID)

	return &StartResult{
		RunID: runID, SessionID: conv.SessionID, Status: status,
		StopReason: loopResult.StopReason, Output: loopResult.Content,
		Steps: loopResult.Steps, Tokens: loopResult.Tokens,
		PlanMode: planMode,
	}, nil
}

// finishPlanRun 收尾一次 Plan-and-Execute run：消息、run 状态、plan_json、指标、异步记忆。
func (s *Service) finishPlanRun(req StartRequest, conv *models.Conversation, runID string, result *planpkg.Result, start time.Time) *StartResult {
	durationMs := time.Since(start).Milliseconds()

	if _, err := s.saveMessage(conv.SessionID, req.Username, "assistant", result.Answer, ""); err != nil {
		logrus.Warnf("[agent-run] save assistant message failed: %v", err)
	}

	status := models.AgentRunStatusCompleted
	switch result.Status {
	case planpkg.ResultPartial:
		status = models.AgentRunStatusPartial
	case planpkg.ResultFailed:
		status = models.AgentRunStatusFailed
	case planpkg.ResultCanceled:
		status = models.AgentRunStatusCanceled
	}
	errMsg := ""
	if result.Status == planpkg.ResultFailed {
		errMsg = result.StopReason
	}
	_ = s.repository().FinishAgentRun(runID, status, result.StopReason,
		result.Answer, errMsg, result.Steps, result.Tokens)

	if result.Plan != nil {
		if raw, err := json.Marshal(result.Plan); err == nil {
			if err := s.repository().UpdateAgentRunPlan(runID, string(raw)); err != nil {
				logrus.Warnf("[agent-run] update plan_json failed: %v", err)
			}
		}
	}

	metric.GetMetricService().Record(models.MetricLog{
		Module: "agent", Event: "run_plan_completed",
		Success:   status == models.AgentRunStatusCompleted || status == models.AgentRunStatusPartial,
		SessionID: conv.SessionID, DurationMs: durationMs, ErrorType: errMsg,
		Extra: fmt.Sprintf(`{"status":%q,"stop_reason":%q,"tasks":%d,"steps":%d,"tokens":%d}`,
			result.Status, result.StopReason, len(result.Tasks), result.Steps, result.Tokens),
	})

	agentctx.CompressAsync(conv.SessionID, req.Username, s.cfg.Context.CompressThresholdTokens)
	agentctx.ConsolidateAsync(req.Username, conv.SessionID)

	return &StartResult{
		RunID: runID, SessionID: conv.SessionID, Status: status,
		StopReason: result.StopReason, Output: result.Answer,
		Steps: result.Steps, Tokens: result.Tokens,
		PlanMode: "plan",
	}
}

// contextPromptBudget 计算"前缀 + 短期消息"可用 token：
// 窗口 - 输出预留 - 安全余量 - 工具/协议预留，再受软预算限制。
func contextPromptBudget(c config.ContextConfig) int {
	budget := c.ModelWindowTokens - c.MaxOutputTokens - c.SafetyMarginTokens - c.ReserveTokens
	if c.SoftLimitTokens > 0 && budget > c.SoftLimitTokens {
		budget = c.SoftLimitTokens
	}
	if budget <= 0 {
		budget = 12000
	}
	return budget
}

// buildContextHint 从四层上下文里提取 system 部分（persona/画像/摘要）给规划器，
// 不把近期对话塞进去，避免规划器被历史细节带偏。
func buildContextHint(assembled *agentctx.Assembled) string {
	if assembled == nil {
		return ""
	}
	var b strings.Builder
	for _, m := range assembled.Messages {
		if m.Role != "system" {
			continue
		}
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return truncateRunes(b.String(), 2000)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// saveMessage 写 MySQL（分配 sequence）并更新 Redis 热列表与会话计数
func (s *Service) saveMessage(sessionID, userID, role, content, metadata string) (*models.Message, error) {
	msg := &models.Message{
		SessionID: sessionID, UserID: userID, Role: role,
		Content: content, Metadata: metadata,
	}
	if err := s.repository().CreateMessage(msg); err != nil {
		return nil, err
	}
	if err := repositories.PushRecentMessage(context.Background(), sessionID, msg); err != nil {
		logrus.Warnf("[agent-run] push recent message failed: %v", err)
	}
	count, err := s.repository().CountMessages(sessionID, userID)
	if err == nil {
		_ = s.repository().UpdateConversationMessageCount(sessionID, userID, int(count))
	}
	return msg, nil
}

// ensureConversation 校验或创建 Agent 会话
func (s *Service) ensureConversation(req StartRequest) (*models.Conversation, error) {
	if req.SessionID != "" {
		conv, err := s.repository().FindConversationBySessionAndUser(req.SessionID, req.Username)
		if err != nil {
			return nil, ecode.Wrap(err, "查询会话失败")
		}
		if conv == nil {
			return nil, ecode.New(ecode.ErrSessionNotFound, "会话不存在或无权限")
		}
		return conv, nil
	}

	conv := &models.Conversation{
		SessionID: uuid.NewString(),
		UserID:    req.Username,
		Scene:     models.ConversationSceneAgent,
		AgentID:   req.AgentID,
		Title:     truncateTitle(req.Input),
		Status:    models.ConversationStatusActive,
	}
	if err := s.repository().CreateConversation(conv); err != nil {
		return nil, ecode.Wrap(err, "创建会话失败")
	}
	return conv, nil
}

// stepRecorder 把循环步骤写入 MySQL。
// plan 子任务共用 runID，但 seq 非空时由 run 级原子序号统一分配 StepIndex，
// 保证并行任务和失败重试不会撞 (run_id, step_index) 唯一键。
type stepRecorder struct {
	repo   runRepository
	runID  string
	taskID string
	seq    *atomic.Int64
}

func (r *stepRecorder) repository() runRepository {
	if r.repo != nil {
		return r.repo
	}
	return mysqlRunRepository{}
}

func (r *stepRecorder) prepare(step *models.AgentStep) {
	step.RunID = r.runID
	if r.taskID != "" {
		step.TaskID = r.taskID
	}
	if r.seq != nil {
		step.StepIndex = int(r.seq.Add(1))
	}
}

func (r *stepRecorder) RecordStep(step *models.AgentStep) error {
	r.prepare(step)
	return r.repository().CreateAgentStep(step)
}

func validate(req StartRequest) error {
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.UserID) == "" {
		return ecode.New(ecode.NotLogin, "缺少用户身份")
	}
	if strings.TrimSpace(req.Input) == "" {
		return ecode.New(ecode.ErrAgentInvalidInput, "输入不能为空")
	}
	if len([]rune(req.Input)) > 4000 {
		return ecode.New(ecode.ErrAgentInvalidInput, "输入过长（上限 4000 字符）")
	}
	return nil
}

func dedupKey(req StartRequest) string {
	sum := sha256.Sum256([]byte(req.Username + "|" + req.SessionID + "|" + req.Input))
	return "agent:run:" + hex.EncodeToString(sum[:12])
}

func truncateTitle(input string) string {
	r := []rune(strings.TrimSpace(input))
	if len(r) > 30 {
		return string(r[:30])
	}
	if len(r) == 0 {
		return "新会话"
	}
	return string(r)
}

// ============================================================
// 流式幂等锁（SingleFlight 只广播最终结果，无法复用 chunk）
// ============================================================

var releaseRunLockScript = redis.NewScript(`
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("del", KEYS[1])
	end
	return 0
`)

func acquireRunLock(ctx context.Context, key string) (func(), error) {
	token := uuid.NewString()
	locked, err := repositories.RedisClient.SetNX(ctx, key, token, 2*time.Minute).Result()
	if err != nil {
		return nil, ecode.Wrap(err, "获取 run 锁失败")
	}
	if !locked {
		return nil, ecode.New(ecode.ErrAgentRunBusy, "相同请求正在处理中，请稍后重试")
	}
	return func() {
		_ = releaseRunLockScript.Run(context.Background(), repositories.RedisClient, []string{key}, token).Err()
	}, nil
}
