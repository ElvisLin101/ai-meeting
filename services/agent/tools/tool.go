package tools

import (
	"ai-meeting/clients"
	"context"
	"encoding/json"
)

// ============================================================
// Agent 工具框架
//
// 每个工具声明 JSON Schema, 由 Executor 统一做参数校验、超时控制、
// panic 恢复、结果截断与审计。工具实现只关心业务。
// ============================================================

// Context 工具执行上下文（用户/会话/run 归属）
type Context struct {
	UserID    string // JWT user_id（面试模块使用）
	Username  string // JWT username（Agent 会话使用）
	SessionID string
	RunID     string
	AgentID   uint
}

// Result 工具执行结果
type Result struct {
	Content   string
	Truncated bool
	Meta      map[string]interface{}
}

// Tool 工具契约
type Tool interface {
	Name() string
	Description() string
	// Parameters 返回 JSON Schema（object 类型，包含 properties/required）
	Parameters() json.RawMessage
	Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error)
}

// ToolMeta 描述工具的执行特征。
//
// 零值表示「只读=false、可并发=false」：未显式声明的工具默认不参与并行，
// 避免未知副作用并发执行。只读且无共享状态的工具应显式声明 ConcurrencySafe。
type ToolMeta struct {
	ReadOnly        bool
	ConcurrencySafe bool
}

// MetaProvider 是 Tool 的可选扩展，用于描述并发/只读特征。
type MetaProvider interface {
	Meta() ToolMeta
}

// ToolMetaOf 读取工具元数据；未实现 MetaProvider 时返回保守默认值。
func ToolMetaOf(t Tool) ToolMeta {
	if p, ok := t.(MetaProvider); ok {
		return p.Meta()
	}
	return ToolMeta{}
}

// Registry 工具注册表
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry 创建空注册表
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册工具（同名覆盖）
func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Get 按名取工具
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Names 返回工具名列表（注册顺序）
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// Defs 生成 OpenAI tools 定义
func (r *Registry) Defs() []clients.ToolDef {
	defs := make([]clients.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, clients.ToolDef{
			Type: "function",
			Function: clients.ToolFunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}
