package agentctx

import "time"

// ============================================================
// 四层上下文的数据结构
//
// L1 短期: 近期全量对话（Redis List, MySQL 回填）
// L2 远期: 压缩摘要（Redis, MySQL 回填）
// L3 跨会话: 用户画像（结构化 JSON）
// L4 自我定位: Agent persona + 低频自省
// ============================================================

// UserProfileData 用户画像（结构化）
type UserProfileData struct {
	Preferences map[string]string      `json:"preferences,omitempty"` // 沟通偏好等
	Goals       []string               `json:"goals,omitempty"`       // 求职目标
	Constraints []string               `json:"constraints,omitempty"` // 约束（时间/地域/技能）
	Background  map[string]interface{} `json:"background,omitempty"`  // 背景（岗位/技能栈）
	Confidence  map[string]float64     `json:"confidence,omitempty"`  // 各字段置信度
	Version     int                    `json:"version"`
	UpdatedAt   string                 `json:"updated_at"`
}

// AgentIdentityData Agent 自我定位（静态 persona 为主）
type AgentIdentityData struct {
	Name             string   `json:"name"`
	Role             string   `json:"role"`
	Capabilities     []string `json:"capabilities,omitempty"`
	Boundaries       []string `json:"boundaries,omitempty"`
	InteractionStyle string   `json:"interaction_style,omitempty"`
	SelfReflection   string   `json:"self_reflection,omitempty"` // 低频更新, 防人设漂移
	Version          int      `json:"version"`
}

// DefaultIdentity 默认 Agent 自我定位（未配置时使用）
func DefaultIdentity() *AgentIdentityData {
	return &AgentIdentityData{
		Name: "CareerAgent",
		Role: "求职与面试准备助手",
		Capabilities: []string{
			"解析简历", "检索公开面经与岗位信息", "生成学习计划",
			"组织结构化模拟面试", "在沙箱内整理笔记与文件",
		},
		Boundaries: []string{
			"不编造未经验证的公司信息", "不修改沙箱外的文件",
			"不代替用户做最终职业决策",
		},
		InteractionStyle: "直接、务实、先给结论再给依据",
		Version:          1,
	}
}

// BuildInput 上下文拼装输入
type BuildInput struct {
	UserID    string
	SessionID string
	AgentID   uint
	// Query 是当前用户消息；用于从历史中剔除重复的当前消息，避免重复计费。
	Query string
}

// Assembled 拼装结果
type Assembled struct {
	Messages          []Message
	UsedSummary       bool
	UsedProfile       bool
	RecentMessages    int
	DroppedMessages   int
	TotalTokens       int
	CoveredThroughSeq int64
	// TotalBytes 仅保留给旧日志/调试使用；预算判断一律用 TotalTokens。
	TotalBytes int
}

// Message 与 clients.ToolMessage 对齐的轻量消息
type Message struct {
	Role    string
	Content string
}

// nowString 当前时间字符串
func nowString() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
