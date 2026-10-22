package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ai-meeting/clients"
)

// ReplanInput 是 replanner 的决策输入。它只看到当前计划、结果和触发原因，
// 不直接修改计划；修改由 Service 在轮次边界串行应用。
type ReplanInput struct {
	Goal        string
	ContextHint string
	Plan        *Plan
	Results     map[string]TaskResult
	Reasons     []string
	MaxNewTasks int
	AgentID     uint
}

// Replanner 产出一个增量 PlanDiff，而不是整张新计划。
type Replanner interface {
	Propose(ctx context.Context, in ReplanInput) (*PlanDiff, error)
}

// callReplannerLLM 抽成变量便于单测注入假模型。
var callReplannerLLM = clients.CallConfiguredAIChat

// LLMReplanner 用一次结构化 LLM 调用生成计划增量。
type LLMReplanner struct{}

// NewLLMReplanner 创建 LLM replanner。
func NewLLMReplanner() *LLMReplanner { return &LLMReplanner{} }

// Propose 生成 diff。解析失败或无变化时返回 error/空 diff，由上层决定停止 replan。
func (r *LLMReplanner) Propose(ctx context.Context, in ReplanInput) (*PlanDiff, error) {
	if in.Plan == nil {
		return nil, errors.New("nil plan")
	}
	if in.MaxNewTasks <= 0 {
		in.MaxNewTasks = 1
	}

	type taskView struct {
		ID        string   `json:"id"`
		Type      TaskType `json:"type"`
		Goal      string   `json:"goal"`
		Tool      string   `json:"tool,omitempty"`
		DependsOn []string `json:"depends_on"`
		Status    string   `json:"status"`
		Result    string   `json:"result,omitempty"`
		Error     string   `json:"error,omitempty"`
		Replaces  string   `json:"replaces,omitempty"`
	}
	views := make([]taskView, 0, len(in.Plan.Tasks))
	for _, t := range in.Plan.Tasks {
		r := in.Results[t.ID]
		views = append(views, taskView{
			ID: t.ID, Type: t.Type, Goal: t.Goal, Tool: t.Tool,
			DependsOn: t.DependsOn, Status: string(r.Status),
			Result: truncateRunes(r.Summary, 800), Error: truncateRunes(r.Error, 300),
			Replaces: t.Replaces,
		})
	}
	planJSON, _ := json.Marshal(views)

	prompt := fmt.Sprintf(`用户目标：
%s

用户上下文（画像/摘要，可能为空）：
%s

当前计划（含每个任务的最终状态与结果摘要）：
%s

触发 replan 的原因：
- %s

规则：
- 只能修改状态为 pending / failed 的任务；done 的任务绝对不能修改或删除。
- 输出 JSON 对象: {"reason":"...","add":[...],"update":[...],"remove":["id"]}。
- 如果不需要修改，输出 {"reason":"no_change"}，不要硬造任务。
- add/update 的 task 字段与计划任务一致: id, type, goal, tool, params, depends_on, acceptance。
- 新增任务最多 %d 个；id 唯一且不能与现有 id 冲突；depends_on 只能引用现有或本次新增的任务；不能成环。
- 替代失败任务时使用新 id，并设置 "replaces":"旧任务id"；不要复用旧 id。
- tool 任务只能使用只读工具；params 必须是 JSON 对象。
- 不要重复已完成的任务；同语义任务会被系统按幂等 key 复用结果。
- 只输出 JSON，不要输出解释或 Markdown 代码块。`,
		strings.TrimSpace(in.Goal),
		truncateRunes(strings.TrimSpace(in.ContextHint), 1200),
		string(planJSON),
		strings.Join(in.Reasons, "\n- "),
		in.MaxNewTasks,
	)

	raw, err := callReplannerLLM(ctx, in.AgentID, []clients.PromptMessage{
		{Role: "system", Content: "你是计划修复器。只输出合法 JSON，不执行用户任务，不调用工具。"},
		{Role: "user", Content: prompt},
	}, 0.2)
	if err != nil {
		return nil, fmt.Errorf("replanner llm: %w", err)
	}

	var diff PlanDiff
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &diff); err != nil {
		return nil, fmt.Errorf("parse replan diff: %w", err)
	}
	if strings.TrimSpace(diff.Reason) == "" {
		diff.Reason = "replan"
	}
	// 模型不能指定幂等 key，避免错误去重；服务端会根据任务语义重新计算。
	for i := range diff.Add {
		diff.Add[i].IdempotencyKey = ""
	}
	for i := range diff.Update {
		diff.Update[i].IdempotencyKey = ""
	}
	if strings.EqualFold(diff.Reason, "no_change") ||
		(len(diff.Add) == 0 && len(diff.Update) == 0 && len(diff.Remove) == 0) {
		diff.Reason = "no_change"
	}
	return &diff, nil
}
