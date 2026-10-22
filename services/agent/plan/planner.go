package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ai-meeting/clients"
	"ai-meeting/services/agent/tools"
)

// ErrNoPlan 表示这次请求不需要计划，调用方应回退到线性 ReAct。
var ErrNoPlan = errors.New("no plan needed")

// callPlannerLLM 抽成变量便于单测注入假模型。
var callPlannerLLM = clients.CallConfiguredAIChat

// Planner 负责生成、校验任务图；是否进入 Plan 模式由 Router 前置判断。
type Planner struct {
	cfg      Config
	registry *tools.Registry
	router   *Router
}

// NewPlanner 创建规划器。
func NewPlanner(cfg Config, registry *tools.Registry) *Planner {
	cfg = cfg.withDefaults()
	return &Planner{cfg: cfg, registry: registry, router: NewRouter(cfg)}
}

// ShouldPlan 兼容旧调用；实际判断由 Router 完成。
func (p *Planner) ShouldPlan(query string) bool {
	return p.Decide(query).UsePlan
}

// Decide 返回前置路由决策，供调用方记录/上报。
func (p *Planner) Decide(query string) RouteDecision {
	if p == nil || p.router == nil {
		return RouteDecision{Mode: "direct", Reason: "router unavailable"}
	}
	return p.router.Decide(query)
}

// Plan 调用规划模型生成任务图，并做结构与安全校验。
// 返回 ErrNoPlan 表示不需要计划；其它错误表示规划失败，调用方应回退。
func (p *Planner) Plan(ctx context.Context, in Input) (*Plan, error) {
	if p == nil || p.registry == nil {
		return nil, errors.New("planner unavailable")
	}

	userPrompt := fmt.Sprintf(`用户目标：
%s

上下文（用户画像/长期摘要，可能为空，仅作规划参考）：
%s

可用工具：
%s

规则：
- 输出 JSON 对象: {"tasks":[...]}，不要输出解释或 Markdown 代码块。
- 每个任务字段: id, type, goal, depends_on, acceptance。
- type 只能是 agent / tool / llm。
- type=tool 时必须给 tool 和 params；tool 必须是上面的只读工具，否则不要用 plan。
- type=agent 用于需要多步推理或调用工具的子任务。
- type=llm 用于只基于上游结果做一次文本推理/总结。
- 任务数 1~%d；简单目标只输出 1 个 agent 任务。
- depends_on 只能引用本计划内的 id，不能成环。
- 不要编造工具，不要把有副作用的操作并行安排。
- acceptance 写可检查的完成标准，最多 2 条。`,
		strings.TrimSpace(in.Query),
		truncateRunes(strings.TrimSpace(in.ContextHint), 2000),
		p.toolCatalog(),
		p.cfg.MaxTasks,
	)

	raw, err := callPlannerLLM(ctx, in.AgentID, []clients.PromptMessage{
		{Role: "system", Content: "你是任务规划器。只输出合法 JSON，不执行用户目标，不调用工具。"},
		{Role: "user", Content: userPrompt},
	}, 0.2)
	if err != nil {
		return nil, fmt.Errorf("planner llm: %w", err)
	}

	var pPlan Plan
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &pPlan); err != nil {
		return nil, fmt.Errorf("parse plan json: %w", err)
	}
	pPlan.Goal = strings.TrimSpace(in.Query)
	// 模型输出只作为任务语义来源：状态、结果、幂等 key 全部由服务端管理，
	// 防止模型伪造“已完成”或错误去重。
	for i := range pPlan.Tasks {
		t := &pPlan.Tasks[i]
		t.IdempotencyKey = ""
		t.Status = StatusPending
		t.Attempts = 0
		t.Result = ""
		t.Error = ""
		t.Steps = 0
		t.Tokens = 0
		t.DurationMs = 0
	}
	pPlan.Normalize()

	if len(pPlan.Tasks) == 0 {
		return nil, ErrNoPlan
	}
	// 只有一个 agent 任务时，计划没有增量价值，交给线性 ReAct 更便宜。
	if len(pPlan.Tasks) == 1 && pPlan.Tasks[0].Type == TaskTypeAgent {
		return nil, ErrNoPlan
	}
	if err := pPlan.Validate(p.cfg.MaxTasks, p.registry); err != nil {
		return nil, err
	}
	if err := pPlan.EnsureIdempotencyKeys(); err != nil {
		return nil, err
	}
	return &pPlan, nil
}

// Validate 校验任务图：id、类型、工具、依赖、环、任务数。
func (p *Plan) Validate(maxTasks int, registry *tools.Registry) error {
	if p == nil || len(p.Tasks) == 0 {
		return errors.New("plan has no tasks")
	}
	if maxTasks > 0 && len(p.Tasks) > maxTasks {
		return fmt.Errorf("plan has %d tasks, max %d", len(p.Tasks), maxTasks)
	}

	byID := make(map[string]Task, len(p.Tasks))
	for _, t := range p.Tasks {
		if strings.TrimSpace(t.ID) == "" {
			return errors.New("task id is empty")
		}
		if _, exists := byID[t.ID]; exists {
			return fmt.Errorf("duplicate task id: %s", t.ID)
		}
		byID[t.ID] = t
	}

	for _, t := range p.Tasks {
		switch t.Type {
		case TaskTypeAgent, TaskTypeTool, TaskTypeLLM:
		default:
			return fmt.Errorf("task %s has invalid type %q", t.ID, t.Type)
		}
		if t.Type != TaskTypeTool && strings.TrimSpace(t.Goal) == "" {
			return fmt.Errorf("task %s goal is empty", t.ID)
		}
		if t.Type == TaskTypeTool {
			if strings.TrimSpace(t.Tool) == "" {
				return fmt.Errorf("tool task %s missing tool", t.ID)
			}
			if registry == nil {
				return errors.New("tool registry unavailable")
			}
			tool, ok := registry.Get(t.Tool)
			if !ok {
				return fmt.Errorf("tool task %s uses unknown tool %q", t.ID, t.Tool)
			}
			// 计划内允许并行，因此 tool 任务只允许只读工具；
			// 有副作用的操作交给 agent 任务或线性 ReAct。
			if !tools.ToolMetaOf(tool).ReadOnly {
				return fmt.Errorf("tool task %s uses non-readonly tool %q", t.ID, t.Tool)
			}
		}

		depSeen := make(map[string]bool, len(t.DependsOn))
		for _, dep := range t.DependsOn {
			if dep == t.ID {
				return fmt.Errorf("task %s depends on itself", t.ID)
			}
			if depSeen[dep] {
				return fmt.Errorf("task %s has duplicate dependency %s", t.ID, dep)
			}
			depSeen[dep] = true
			if _, ok := byID[dep]; !ok {
				return fmt.Errorf("task %s depends on unknown task %s", t.ID, dep)
			}
		}
	}

	// 环检测（Kahn）
	indeg := make(map[string]int, len(p.Tasks))
	adj := make(map[string][]string, len(p.Tasks))
	for _, t := range p.Tasks {
		indeg[t.ID] = len(t.DependsOn)
		for _, dep := range t.DependsOn {
			adj[dep] = append(adj[dep], t.ID)
		}
	}
	queue := make([]string, 0, len(p.Tasks))
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	seen := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		seen++
		for _, down := range adj[id] {
			indeg[down]--
			if indeg[down] == 0 {
				queue = append(queue, down)
			}
		}
	}
	if seen != len(p.Tasks) {
		return errors.New("plan contains a cycle")
	}
	return nil
}

// toolCatalog 生成规划器可读的工具清单（名称 + 描述 + 参数 schema）。
func (p *Planner) toolCatalog() string {
	if p.registry == nil {
		return "（无可用工具）"
	}
	var b strings.Builder
	for _, name := range p.registry.Names() {
		t, ok := p.registry.Get(name)
		if !ok {
			continue
		}
		marker := "有副作用"
		if tools.ToolMetaOf(t).ReadOnly {
			marker = "只读"
		}
		fmt.Fprintf(&b, "- %s [%s]: %s\n  参数: %s\n", name, marker, t.Description(), string(t.Parameters()))
	}
	return truncateRunes(b.String(), 4000)
}

// extractJSONObject 从模型输出中截取首个完整 JSON 对象。
func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return text
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
