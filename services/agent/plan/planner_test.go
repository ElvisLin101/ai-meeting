package plan

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ai-meeting/clients"
	"ai-meeting/services/agent/tools"
)

type fakePlanTool struct {
	name     string
	readOnly bool
}

func (f *fakePlanTool) Name() string                { return f.name }
func (f *fakePlanTool) Description() string         { return "fake tool" }
func (f *fakePlanTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakePlanTool) Execute(context.Context, *tools.Context, json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (f *fakePlanTool) Meta() tools.ToolMeta {
	return tools.ToolMeta{ReadOnly: f.readOnly, ConcurrencySafe: f.readOnly}
}

func testRegistry() *tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(&fakePlanTool{name: "doc_search", readOnly: true})
	reg.Register(&fakePlanTool{name: "file_write", readOnly: false})
	return reg
}

func newTestPlanner() *Planner {
	return NewPlanner(Config{Enabled: true, MaxTasks: 6, LongQueryRunes: 40}, testRegistry())
}

func withPlannerLLM(t *testing.T, fn func(ctx context.Context, messages []clients.PromptMessage) (string, error)) {
	t.Helper()
	orig := callPlannerLLM
	callPlannerLLM = func(ctx context.Context, aiID uint, messages []clients.PromptMessage, temperature float64) (string, error) {
		return fn(ctx, messages)
	}
	t.Cleanup(func() { callPlannerLLM = orig })
}

func TestShouldPlan(t *testing.T) {
	p := newTestPlanner()
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"resume rewrite", "帮我改改简历", true},
		{"multi-step report", "分析三家公司的面经并生成对比报告", true},
		{"explicit sequence", "先分析简历，再对比 JD，最后生成复习计划", true},
		{"greeting", "你好", false},
		{"weather", "今天天气怎么样？", false},
		{"resume template", "简历模板怎么排版？", false},
		{"single summary", "帮我总结一下这段文字", false},
		{"single analysis", "分析这段代码", false},
	}
	for _, tc := range cases {
		if got := p.ShouldPlan(tc.query); got != tc.want {
			t.Errorf("%s: ShouldPlan(%q) = %v, want %v", tc.name, tc.query, got, tc.want)
		}
	}

	disabled := NewPlanner(Config{Enabled: false}, testRegistry())
	if disabled.ShouldPlan("帮我分析并生成报告") {
		t.Fatal("disabled planner should not trigger")
	}
}

func TestRouterModes(t *testing.T) {
	always := NewRouter(Config{Enabled: true, RouteMode: "always"})
	if d := always.Decide("你好"); !d.UsePlan || d.Mode != "plan" {
		t.Fatalf("always mode should use plan, got %+v", d)
	}
	never := NewRouter(Config{Enabled: true, RouteMode: "never"})
	if d := never.Decide("分析三家公司的面经并生成对比报告"); d.UsePlan || d.Mode != "direct" {
		t.Fatalf("never mode should skip plan, got %+v", d)
	}
}

func TestPlannerParsesValidPlan(t *testing.T) {
	withPlannerLLM(t, func(context.Context, []clients.PromptMessage) (string, error) {
		return "```json\n" + `{
			"tasks": [
				{"id":"t1","type":"tool","tool":"doc_search","params":{"query":"字节后端"},"depends_on":[]},
				{"id":"t2","type":"llm","goal":"比较 t1 的结果","depends_on":["t1"],"acceptance":["给出结论"]}
			]
		}` + "\n```", nil
	})

	p, err := newTestPlanner().Plan(context.Background(), Input{Query: "分析字节后端岗位", AgentID: 1})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(p.Tasks))
	}
	if p.Tasks[0].Type != TaskTypeTool || p.Tasks[0].Tool != "doc_search" {
		t.Fatalf("unexpected tool task: %+v", p.Tasks[0])
	}
	if p.Tasks[1].Status != StatusPending {
		t.Fatalf("task status should default to pending, got %s", p.Tasks[1].Status)
	}
}

func TestPlannerRejectsCycle(t *testing.T) {
	withPlannerLLM(t, func(context.Context, []clients.PromptMessage) (string, error) {
		return `{"tasks":[
			{"id":"t1","type":"agent","goal":"a","depends_on":["t2"]},
			{"id":"t2","type":"agent","goal":"b","depends_on":["t1"]}
		]}`, nil
	})

	_, err := newTestPlanner().Plan(context.Background(), Input{Query: "分析并生成报告", AgentID: 1})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestPlannerRejectsNonReadOnlyTool(t *testing.T) {
	withPlannerLLM(t, func(context.Context, []clients.PromptMessage) (string, error) {
		return `{"tasks":[
			{"id":"t1","type":"tool","tool":"file_write","params":{"path":"a.md","content":"x"},"depends_on":[]},
			{"id":"t2","type":"agent","goal":"总结","depends_on":["t1"]}
		]}`, nil
	})

	_, err := newTestPlanner().Plan(context.Background(), Input{Query: "分析并写文件", AgentID: 1})
	if err == nil || !strings.Contains(err.Error(), "non-readonly") {
		t.Fatalf("want non-readonly error, got %v", err)
	}
}

func TestPlannerSingleAgentReturnsNoPlan(t *testing.T) {
	withPlannerLLM(t, func(context.Context, []clients.PromptMessage) (string, error) {
		return `{"tasks":[{"id":"t1","type":"agent","goal":"完成用户目标","depends_on":[]}]}`, nil
	})

	_, err := newTestPlanner().Plan(context.Background(), Input{Query: "分析并生成报告", AgentID: 1})
	if !errors.Is(err, ErrNoPlan) {
		t.Fatalf("want ErrNoPlan, got %v", err)
	}
}

func TestPlannerInvalidJSON(t *testing.T) {
	withPlannerLLM(t, func(context.Context, []clients.PromptMessage) (string, error) {
		return "这不是 JSON", nil
	})

	_, err := newTestPlanner().Plan(context.Background(), Input{Query: "分析并生成报告", AgentID: 1})
	if err == nil {
		t.Fatal("invalid JSON should fail")
	}
}

func TestPlanValidateDuplicateID(t *testing.T) {
	p := &Plan{Tasks: []Task{
		{ID: "t1", Type: TaskTypeAgent, Goal: "a"},
		{ID: "t1", Type: TaskTypeAgent, Goal: "b"},
	}}
	if err := p.Validate(6, testRegistry()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate error, got %v", err)
	}
}
