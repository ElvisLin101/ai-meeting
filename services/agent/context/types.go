package agentctx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ============================================================
// 四层上下文的数据结构
//
// L1 短期: 近期全量对话（Redis List, MySQL 回填）
// L2 远期: 压缩摘要（Redis, MySQL 回填）
// L3 跨会话: 用户画像（结构化 JSON）
// L4 自我定位: Agent persona + 低频自省
// ============================================================

// ProfileFact 是结构化画像中的一条事实。
type ProfileFact struct {
	ID         string  `json:"id"`
	Field      string  `json:"field"` // goal / constraint / background / preference / interview
	Key        string  `json:"key,omitempty"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source,omitempty"`
	Quote      string  `json:"quote,omitempty"`
	Status     string  `json:"status,omitempty"` // active / superseded / expired
	UpdatedAt  string  `json:"updated_at,omitempty"`
}

// UserProfileData 用户画像（结构化）
type UserProfileData struct {
	// 兼容旧画像字段；新逻辑以 Facts 为准。
	Preferences map[string]string      `json:"preferences,omitempty"` // 沟通偏好等
	Goals       []string               `json:"goals,omitempty"`       // 求职目标
	Constraints []string               `json:"constraints,omitempty"` // 约束（时间/地域/技能）
	Background  map[string]interface{} `json:"background,omitempty"`  // 背景（岗位/技能栈）
	Confidence  map[string]float64     `json:"confidence,omitempty"`  // 各字段置信度
	Facts       []ProfileFact          `json:"facts,omitempty"`       // 结构化事实
	Version     int                    `json:"version"`
	// LastConsolidatedSeq 是 L2 已经处理到的消息 sequence。
	LastConsolidatedSeq int64  `json:"last_consolidated_seq,omitempty"`
	UpdatedAt           string `json:"updated_at"`
}

// ActiveFacts 返回 active 状态的事实，按 field/key 稳定排序。
func (p *UserProfileData) ActiveFacts() []ProfileFact {
	if p == nil {
		return nil
	}
	out := make([]ProfileFact, 0, len(p.Facts))
	for _, f := range p.Facts {
		if f.Status == "" || f.Status == "active" {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Field == out[j].Field {
			return out[i].Key < out[j].Key
		}
		return out[i].Field < out[j].Field
	})
	return out
}

// NormalizeProfile 补齐 ID/status/时间，并把旧画像字段迁移成 Facts。
// 旧字段保留，避免旧调用方读取时丢数据；新逻辑只写 Facts。
func NormalizeProfile(p *UserProfileData) {
	if p == nil {
		return
	}
	now := nowString()
	facts := make([]ProfileFact, 0, len(p.Facts)+len(p.Goals)+len(p.Constraints)+len(p.Preferences)+len(p.Background))
	seen := make(map[string]int)

	add := func(f ProfileFact) {
		if strings.TrimSpace(f.Value) == "" {
			return
		}
		if f.Status == "" {
			f.Status = "active"
		}
		if f.Confidence <= 0 {
			f.Confidence = 0.5
		}
		if f.UpdatedAt == "" {
			f.UpdatedAt = now
		}
		if f.ID == "" {
			f.ID = factID(f.Field, f.Key, f.Value)
		}
		key := f.Field + "|" + f.Key + "|" + f.Value
		if idx, ok := seen[key]; ok {
			if f.Confidence > facts[idx].Confidence {
				facts[idx] = f
			}
			return
		}
		seen[key] = len(facts)
		facts = append(facts, f)
	}

	for _, f := range p.Facts {
		add(f)
	}
	// 旧字段迁移：只在 Facts 为空时做，避免重复。
	if len(p.Facts) == 0 {
		for _, g := range p.Goals {
			add(ProfileFact{Field: "goal", Key: "goal", Value: g, Confidence: 0.7})
		}
		for _, c := range p.Constraints {
			add(ProfileFact{Field: "constraint", Key: "constraint", Value: c, Confidence: 0.7})
		}
		for k, v := range p.Preferences {
			add(ProfileFact{Field: "preference", Key: k, Value: v, Confidence: 0.7})
		}
		for k, v := range p.Background {
			add(ProfileFact{Field: "background", Key: k, Value: fmt.Sprint(v), Confidence: 0.6})
		}
	}

	p.Facts = facts
	if p.Version <= 0 {
		p.Version = 1
	}
}

func factID(field, key, value string) string {
	sum := sha256.Sum256([]byte(field + "|" + key + "|" + value))
	return hex.EncodeToString(sum[:8])
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
// Report 是本次上下文装配的可观测快照。
type Report struct {
	Mode              string `json:"mode"`
	BudgetTokens      int    `json:"budget_tokens"`
	UsedTokens        int    `json:"used_tokens"`
	IdentityTokens    int    `json:"identity_tokens"`
	ProfileTokens     int    `json:"profile_tokens"`
	SummaryTokens     int    `json:"summary_tokens"`
	RecentTokens      int    `json:"recent_tokens"`
	RecentMessages    int    `json:"recent_messages"`
	DroppedMessages   int    `json:"dropped_messages"`
	ProfileFacts      int    `json:"profile_facts"`
	UsedProfile       bool   `json:"used_profile"`
	UsedSummary       bool   `json:"used_summary"`
	CoveredThroughSeq int64  `json:"covered_through_seq"`
	GeneratedAt       string `json:"generated_at"`
}

type Assembled struct {
	Messages          []Message
	UsedSummary       bool
	UsedProfile       bool
	RecentMessages    int
	DroppedMessages   int
	TotalTokens       int
	CoveredThroughSeq int64
	Report            Report
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
