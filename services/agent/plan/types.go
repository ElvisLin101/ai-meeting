package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ============================================================
// Plan-and-Execute 领域类型
//
// Plan 是服务端拥有的任务图：任务有依赖、验收标准和预算；
// ReAct 是每个任务的执行器，这里不实现 ReAct 本身。
// ============================================================

// Config 控制计划生成与执行。
type Config struct {
	Enabled          bool          // 是否启用 plan 编排
	MaxTasks         int           // 单次计划最大任务数
	MaxParallel      int           // 计划内最大并行任务数
	TaskTimeout      time.Duration // 单个任务超时
	TotalTimeout     time.Duration // 整个计划（含合成）超时
	MaxStepsPerTask  int           // agent 子任务的最大 ReAct 步数
	LongQueryRunes   int           // 超过该长度视为复杂查询
	ReplanEnabled    bool          // 是否允许 replan
	MaxReplans       int           // 单次 run 最大 replan 次数
	RetryMaxAttempts int           // 任务最大尝试次数（含首次）
	RetryBaseBackoff time.Duration // 重试基础退避
	// RouteMode 控制前置路由: auto（规则打分）/ always（总是 plan）/ never（从不 plan）。
	RouteMode string
	// RouteMinScore 是进入 plan 的最低规则得分。
	RouteMinScore int
	// FastPathRunes 是短查询快速通道阈值；短且无强信号时直接走 ReAct。
	FastPathRunes int
}

// withDefaults 补齐零值，避免配置漏填导致计划无法运行。
func (c Config) withDefaults() Config {
	if c.MaxTasks <= 0 {
		c.MaxTasks = 6
	}
	if c.MaxParallel <= 0 {
		c.MaxParallel = 2
	}
	if c.TaskTimeout <= 0 {
		c.TaskTimeout = 60 * time.Second
	}
	if c.TotalTimeout <= 0 {
		c.TotalTimeout = 180 * time.Second
	}
	if c.MaxStepsPerTask <= 0 {
		c.MaxStepsPerTask = 4
	}
	if c.LongQueryRunes <= 0 {
		c.LongQueryRunes = 40
	}
	if c.MaxReplans <= 0 {
		c.MaxReplans = 2
	}
	if c.RetryMaxAttempts <= 0 {
		c.RetryMaxAttempts = 2
	}
	if c.RetryBaseBackoff <= 0 {
		c.RetryBaseBackoff = 500 * time.Millisecond
	}
	if strings.TrimSpace(c.RouteMode) == "" {
		c.RouteMode = "auto"
	}
	if c.RouteMinScore <= 0 {
		c.RouteMinScore = 2
	}
	if c.FastPathRunes <= 0 {
		c.FastPathRunes = 24
	}
	return c
}

// TaskType 任务执行方式。
type TaskType string

const (
	// TaskTypeAgent 用一个独立 ReAct 循环完成子任务（可调用工具）。
	TaskTypeAgent TaskType = "agent"
	// TaskTypeTool 直接调用一次工具，适合确定性、单工具的步骤。
	TaskTypeTool TaskType = "tool"
	// TaskTypeLLM 不做工具调用，只基于上游结果做一次文本推理。
	TaskTypeLLM TaskType = "llm"
)

// TaskStatus 任务状态。
type TaskStatus string

const (
	StatusPending  TaskStatus = "pending"
	StatusRunning  TaskStatus = "running"
	StatusDone     TaskStatus = "done"
	StatusFailed   TaskStatus = "failed"
	StatusSkipped  TaskStatus = "skipped"
	StatusCanceled TaskStatus = "canceled"
)

// Task 是计划中的一个节点。
type Task struct {
	ID         string                 `json:"id"`
	Type       TaskType               `json:"type"`
	Goal       string                 `json:"goal"`
	Tool       string                 `json:"tool,omitempty"`
	Params     map[string]interface{} `json:"params,omitempty"`
	DependsOn  []string               `json:"depends_on"`
	Acceptance []string               `json:"acceptance,omitempty"`
	MaxSteps   int                    `json:"max_steps,omitempty"`

	// IdempotencyKey 由任务语义（类型/工具/目标/参数/上游 key）决定。
	// replan 重新加入同语义任务时复用结果；内容变化则生成新 key。
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	// MaxAttempts 是包含首次执行在内的最大尝试次数；<=0 时使用调度器默认值。
	MaxAttempts int `json:"max_attempts,omitempty"`
	// Attempts 是已执行次数，持久化在 plan_json 中用于审计。
	Attempts int `json:"attempts,omitempty"`
	// Replaces 标记该任务是 replan 为替代某个失败任务而新增的。
	Replaces string `json:"replaces,omitempty"`

	// 执行结果，持久化到 plan_json，便于回放。
	Status     TaskStatus `json:"status,omitempty"`
	Result     string     `json:"result,omitempty"`
	Error      string     `json:"error,omitempty"`
	Steps      int        `json:"steps,omitempty"`
	Tokens     int        `json:"tokens,omitempty"`
	DurationMs int64      `json:"duration_ms,omitempty"`
}

// Plan 是一次 run 的任务图。
type Plan struct {
	Version     int    `json:"version"`
	ReplanCount int    `json:"replan_count,omitempty"`
	Goal        string `json:"goal"`
	Tasks       []Task `json:"tasks"`
}

// Normalize 补齐默认值，保证后续校验/执行拿到一致的结构。
func (p *Plan) Normalize() {
	if p == nil {
		return
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	for i := range p.Tasks {
		t := &p.Tasks[i]
		t.ID = strings.TrimSpace(t.ID)
		if t.Type == "" {
			t.Type = TaskTypeAgent
		}
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}
		if t.Params == nil {
			t.Params = map[string]interface{}{}
		}
		if t.Status == "" {
			t.Status = StatusPending
		}
	}
}

// TaskResult 是调度器产出的单任务结果。
type TaskResult struct {
	ID             string     `json:"id"`
	Status         TaskStatus `json:"status"`
	Summary        string     `json:"summary,omitempty"`
	Error          string     `json:"error,omitempty"`
	Steps          int        `json:"steps,omitempty"`
	Tokens         int        `json:"tokens,omitempty"`
	DurationMs     int64      `json:"duration_ms,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	Attempts       int        `json:"attempts,omitempty"`
	Retryable      bool       `json:"retryable,omitempty"`
	// Reused 表示结果来自幂等账本，没有真正执行。
	Reused bool `json:"reused,omitempty"`
	// ReplanHint 非空时，表示该任务认为计划前提需要调整。
	ReplanHint string `json:"replan_hint,omitempty"`
}

// Result 是一次 plan run 的最终结果。
type Result struct {
	Answer     string
	Plan       *Plan
	Tasks      []TaskResult
	Status     string
	StopReason string
	Steps      int
	Tokens     int
}

// 计划整体状态。
const (
	ResultCompleted = "completed"
	ResultPartial   = "partial"
	ResultFailed    = "failed"
	ResultCanceled  = "canceled"
)

// Input 是 run 服务传给 plan 服务的上下文。
type Input struct {
	Query       string
	ContextHint string
	UserID      string
	Username    string
	SessionID   string
	RunID       string
	AgentID     uint
	// PlanSink 在每个计划版本（初次生成/replan 后）被调用，用于中途持久化 plan_json。
	PlanSink func(*Plan)
}

// ApplyResults 把调度结果写回 Plan，便于持久化和回放。
func (p *Plan) ApplyResults(results []TaskResult) {
	if p == nil {
		return
	}
	byID := make(map[string]TaskResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}
	for i := range p.Tasks {
		r, ok := byID[p.Tasks[i].ID]
		if !ok {
			continue
		}
		p.Tasks[i].Status = r.Status
		p.Tasks[i].Result = r.Summary
		p.Tasks[i].Error = r.Error
		p.Tasks[i].Steps = r.Steps
		p.Tasks[i].Tokens = r.Tokens
		p.Tasks[i].DurationMs = r.DurationMs
		p.Tasks[i].Attempts = r.Attempts
		if r.IdempotencyKey != "" {
			p.Tasks[i].IdempotencyKey = r.IdempotencyKey
		}
	}
}

// PlanDiff 是 replan 的增量修改。只允许改 pending/failed 任务，已完成任务冻结。
type PlanDiff struct {
	Reason string   `json:"reason"`
	Add    []Task   `json:"add,omitempty"`
	Update []Task   `json:"update,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// ApplyDiff 把 diff 应用到一个计划副本上，返回新计划以及需要清除旧结果的 task id。
// 完成的计划不能修改/删除；新增/更新任务的 id 不能与现有任务冲突。
func (p *Plan) ApplyDiff(diff PlanDiff, results map[string]TaskResult) (*Plan, []string, error) {
	if p == nil {
		return nil, nil, errors.New("nil plan")
	}
	byID := make(map[string]Task, len(p.Tasks))
	adj := make(map[string][]string, len(p.Tasks))
	for _, t := range p.Tasks {
		byID[t.ID] = t
		for _, dep := range t.DependsOn {
			adj[dep] = append(adj[dep], t.ID)
		}
	}
	cleared := make([]string, 0, len(diff.Update)+len(diff.Remove))

	for _, id := range diff.Remove {
		if _, ok := byID[id]; !ok {
			return nil, nil, fmt.Errorf("remove unknown task %s", id)
		}
		if results[id].Status == StatusDone {
			return nil, nil, fmt.Errorf("cannot remove completed task %s", id)
		}
		if err := ensureNoCompletedDescendant(id, adj, results); err != nil {
			return nil, nil, err
		}
		delete(byID, id)
		cleared = append(cleared, id)
		cleared = append(cleared, descendants(id, adj)...)
	}

	for _, t := range diff.Update {
		if _, ok := byID[t.ID]; !ok {
			return nil, nil, fmt.Errorf("update unknown task %s", t.ID)
		}
		if results[t.ID].Status == StatusDone {
			return nil, nil, fmt.Errorf("cannot update completed task %s", t.ID)
		}
		if err := ensureNoCompletedDescendant(t.ID, adj, results); err != nil {
			return nil, nil, err
		}
		t.IdempotencyKey = "" // 内容变化后重新计算；语义相同会得到相同 key
		t.Status = StatusPending
		t.Attempts = 0
		t.Result = ""
		t.Error = ""
		t.Steps = 0
		t.Tokens = 0
		t.DurationMs = 0
		byID[t.ID] = t
		cleared = append(cleared, t.ID)
		cleared = append(cleared, descendants(t.ID, adj)...)
	}

	for _, t := range diff.Add {
		if _, ok := byID[t.ID]; ok {
			return nil, nil, fmt.Errorf("add duplicate task %s", t.ID)
		}
		t.IdempotencyKey = "" // 服务端计算，忽略模型提供的 key
		t.Status = StatusPending
		t.Attempts = 0
		t.Result = ""
		t.Error = ""
		t.Steps = 0
		t.Tokens = 0
		t.DurationMs = 0
		byID[t.ID] = t
	}

	tasks := make([]Task, 0, len(byID))
	seen := make(map[string]bool, len(byID))
	// 保留原顺序，新增任务追加在末尾，便于回放。
	for _, t := range p.Tasks {
		if next, ok := byID[t.ID]; ok && !seen[t.ID] {
			tasks = append(tasks, next)
			seen[t.ID] = true
		}
	}
	for _, t := range diff.Add {
		if !seen[t.ID] {
			tasks = append(tasks, byID[t.ID])
			seen[t.ID] = true
		}
	}

	next := &Plan{Version: p.Version, ReplanCount: p.ReplanCount, Goal: p.Goal, Tasks: tasks}
	// 依赖内容变化会改变下游语义，因此全部 key 重算；
	// 未变化的计划内容会得到相同的 key，不影响幂等复用。
	for i := range next.Tasks {
		next.Tasks[i].IdempotencyKey = ""
	}
	next.Normalize()
	if err := next.EnsureIdempotencyKeys(); err != nil {
		return nil, nil, err
	}
	return next, cleared, nil
}

// ensureNoCompletedDescendant 防止 replan 修改一个已有已完成下游的任务：
// 已完成的计划必须冻结，否则下游结果会引用旧输入。
func ensureNoCompletedDescendant(id string, adj map[string][]string, results map[string]TaskResult) error {
	seen := map[string]bool{id: true}
	queue := append([]string(nil), adj[id]...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if results[cur].Status == StatusDone {
			return fmt.Errorf("cannot modify task %s: completed descendant %s must be frozen", id, cur)
		}
		queue = append(queue, adj[cur]...)
	}
	return nil
}

func descendants(id string, adj map[string][]string) []string {
	seen := map[string]bool{id: true}
	out := make([]string, 0)
	queue := append([]string(nil), adj[id]...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		out = append(out, cur)
		queue = append(queue, adj[cur]...)
	}
	return out
}

// EnsureIdempotencyKeys 按依赖递归计算每个任务的幂等 key。
// key 只由语义内容决定，与计划版本无关，因此 replan 重新加入同语义任务时可以复用结果。
func (p *Plan) EnsureIdempotencyKeys() error {
	if p == nil {
		return errors.New("nil plan")
	}
	byID := make(map[string]*Task, len(p.Tasks))
	for i := range p.Tasks {
		byID[p.Tasks[i].ID] = &p.Tasks[i]
	}
	state := make(map[string]int, len(p.Tasks)) // 0=未访问 1=访问中 2=完成

	var calc func(id string) (string, error)
	calc = func(id string) (string, error) {
		t, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("unknown dependency %s", id)
		}
		if state[id] == 1 {
			return "", fmt.Errorf("cycle at task %s", id)
		}
		if state[id] == 2 {
			return t.IdempotencyKey, nil
		}
		state[id] = 1
		if t.IdempotencyKey == "" {
			depKeys := make([]string, 0, len(t.DependsOn))
			for _, dep := range t.DependsOn {
				k, err := calc(dep)
				if err != nil {
					return "", err
				}
				depKeys = append(depKeys, k)
			}
			sort.Strings(depKeys)
			params, _ := json.Marshal(t.Params)
			raw := strings.Join([]string{
				string(t.Type), t.Tool, t.Goal, string(params), strings.Join(depKeys, ","),
			}, "|")
			sum := sha256.Sum256([]byte(raw))
			t.IdempotencyKey = hex.EncodeToString(sum[:16])
		}
		state[id] = 2
		return t.IdempotencyKey, nil
	}

	for _, t := range p.Tasks {
		if _, err := calc(t.ID); err != nil {
			return err
		}
	}
	return nil
}
