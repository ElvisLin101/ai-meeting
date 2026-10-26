package models

import "time"

// ============================================================
// Agent 运行与跨会话结构化数据（MySQL 真相源）
// ============================================================

// Agent run 状态
const (
	AgentRunStatusRunning   = "running"
	AgentRunStatusCompleted = "completed"
	AgentRunStatusPartial   = "partial"
	AgentRunStatusFailed    = "failed"
	AgentRunStatusCanceled  = "canceled"
	AgentRunStatusMaxSteps  = "max_steps"
)

// Agent step 状态
const (
	AgentStepStatusRunning = "running"
	AgentStepStatusOK      = "ok"
	AgentStepStatusFailed  = "failed"
)

// AgentRun 一次 Agent 运行
type AgentRun struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	RunID     string `gorm:"size:64;uniqueIndex;not null" json:"run_id"`
	SessionID string `gorm:"size:64;index;not null" json:"session_id"`
	UserID    string `gorm:"size:128;index;not null" json:"user_id"`
	AgentID   uint   `gorm:"index;default:0" json:"agent_id"`
	Input     string `gorm:"type:longtext" json:"input"`
	Output    string `gorm:"type:longtext" json:"output"`
	// PlanJSON 保存 Plan-and-Execute 的任务图与每个任务的最终状态；线性 ReAct 时为空。
	PlanJSON string `gorm:"type:longtext" json:"plan_json"`
	// ContextReportJSON 保存本次根上下文装配的 token/丢弃/压缩/画像快照。
	ContextReportJSON string     `gorm:"type:longtext" json:"context_report_json"`
	Status            string     `gorm:"size:32;index;not null" json:"status"`
	StopReason        string     `gorm:"size:64" json:"stop_reason"`
	Steps             int        `gorm:"default:0" json:"steps"`
	Tokens            int        `gorm:"default:0" json:"tokens"`
	Error             string     `gorm:"type:text" json:"error"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// TableName 指定表名
func (AgentRun) TableName() string { return "agent_runs" }

// AgentStep 一次 Agent run 的步骤记录（可回放）
type AgentStep struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	RunID       string    `gorm:"size:64;not null;uniqueIndex:uk_run_step,priority:1;index" json:"run_id"`
	TaskID      string    `gorm:"size:64;index" json:"task_id"` // plan 子任务 ID；线性 ReAct 为空
	StepIndex   int       `gorm:"not null;uniqueIndex:uk_run_step,priority:2" json:"step_index"`
	Thought     string    `gorm:"type:longtext" json:"thought"`
	Action      string    `gorm:"size:128" json:"action"` // 工具名, 空表示最终回答
	ActionInput string    `gorm:"type:longtext" json:"action_input"`
	Observation string    `gorm:"type:longtext" json:"observation"`
	Status      string    `gorm:"size:32;not null" json:"status"`
	DurationMs  int64     `gorm:"default:0" json:"duration_ms"`
	Error       string    `gorm:"type:text" json:"error"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 指定表名
func (AgentStep) TableName() string { return "agent_steps" }

// UserProfile 跨会话用户画像（结构化）
// ProfileJSON 结构见 services/agent/context/profile.go 的 UserProfileData。
type UserProfile struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	UserID              string    `gorm:"size:128;uniqueIndex;not null" json:"user_id"`
	ProfileJSON         string    `gorm:"type:longtext" json:"profile_json"`
	SchemaVersion       int       `gorm:"default:1" json:"schema_version"`
	Version             int       `gorm:"default:1" json:"version"`
	LastConsolidatedSeq int64     `gorm:"default:0" json:"last_consolidated_seq"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// TableName 指定表名
func (UserProfile) TableName() string { return "user_profiles" }

// ProfileCandidate 是 L1 规则筛出的长期记忆候选，L2 consolidation 才决定是否写入画像。
type ProfileCandidate struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     string    `gorm:"size:128;index;not null" json:"user_id"`
	SessionID  string    `gorm:"size:64;index" json:"session_id"`
	MessageID  uint      `gorm:"index" json:"message_id"`
	Field      string    `gorm:"size:64;index;not null" json:"field"`
	Value      string    `gorm:"type:text" json:"value"`
	ValueHash  string    `gorm:"size:64;not null;uniqueIndex" json:"value_hash"`
	Confidence float64   `gorm:"default:0" json:"confidence"`
	Quote      string    `gorm:"type:text" json:"quote"`
	Rule       string    `gorm:"size:64" json:"rule"`
	Status     string    `gorm:"size:32;index;default:'pending'" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ProfileCandidate) TableName() string { return "profile_candidates" }

// ProfileConfirmation 保存需要用户确认的模糊/冲突画像项。
type ProfileConfirmation struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     string     `gorm:"size:128;index;not null" json:"user_id"`
	Field      string     `gorm:"size:64" json:"field"`
	Key        string     `gorm:"size:64" json:"key"`
	Value      string     `gorm:"type:text" json:"value"`
	Question   string     `gorm:"type:text" json:"question"`
	Status     string     `gorm:"size:32;index;default:'pending'" json:"status"` // pending/confirmed/rejected
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

// TableName 指定表名
func (ProfileConfirmation) TableName() string { return "profile_confirmations" }

// ProfileEvent 记录每次 L2 consolidation 的审计摘要。
type ProfileEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"size:128;index;not null" json:"user_id"`
	Reason    string    `gorm:"size:255" json:"reason"`
	DiffJSON  string    `gorm:"type:longtext" json:"diff_json"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名
func (ProfileEvent) TableName() string { return "profile_events" }

// AgentIdentity Agent 自我定位（静态 persona + 低频自省）
type AgentIdentity struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AgentID      uint      `gorm:"uniqueIndex;not null" json:"agent_id"`
	IdentityJSON string    `gorm:"type:longtext" json:"identity_json"`
	Version      int       `gorm:"default:1" json:"version"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (AgentIdentity) TableName() string { return "agent_identities" }

// CompressedContextRecord 压缩上下文快照（原 Mongo compressed_contexts 的 MySQL 版）
// 一个会话一行；再次压缩时 UPDATE 同一行。
// Index 是 CoveredThroughSeq：摘要覆盖 sequence <= Index 的消息，
// 短期消息只取 sequence > Index，二者不重叠。
type CompressedContextRecord struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	SessionID         string    `gorm:"size:64;not null;uniqueIndex" json:"session_id"`
	UserID            string    `gorm:"size:128;index" json:"user_id"`
	MemoryScope       string    `gorm:"size:32;index" json:"memory_scope"` // ai/agent
	CompressedContent string    `gorm:"type:longtext" json:"compressed_content"`
	Index             int64     `gorm:"default:0" json:"index"`        // CoveredThroughSeq
	TotalLength       int       `gorm:"default:0" json:"total_length"` // 压缩时未压缩消息的 token 估算
	MessageCount      int       `gorm:"default:0" json:"message_count"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TableName 指定表名
func (CompressedContextRecord) TableName() string { return "compressed_contexts" }
