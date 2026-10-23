package dto

// AgentRunReqDTO 发起一次 Agent run
type AgentRunReqDTO struct {
	SessionID string `json:"session_id"` // 可空, 空则新建会话
	AgentID   uint   `json:"agent_id"`
	Input     string `json:"input" binding:"required"`
}

// AgentRunRespDTO run 摘要
type AgentRunRespDTO struct {
	RunID      string `json:"run_id"`
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	StopReason string `json:"stop_reason"`
	Output     string `json:"output"`
	Steps      int    `json:"steps"`
	Tokens     int    `json:"tokens"`
}

// AgentRunStepRespDTO 单步记录
type AgentRunStepRespDTO struct {
	StepIndex   int    `json:"step_index"`
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	ActionInput string `json:"action_input"`
	Observation string `json:"observation"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error"`
}

// AgentRunDetailRespDTO run 详情（含步骤回放）
type AgentRunDetailRespDTO struct {
	RunID             string                `json:"run_id"`
	SessionID         string                `json:"session_id"`
	Status            string                `json:"status"`
	StopReason        string                `json:"stop_reason"`
	Input             string                `json:"input"`
	Output            string                `json:"output"`
	Steps             int                   `json:"steps"`
	Tokens            int                   `json:"tokens"`
	Error             string                `json:"error"`
	StartedAt         string                `json:"started_at"`
	FinishedAt        string                `json:"finished_at"`
	PlanJSON          string                `json:"plan_json,omitempty"`
	ContextReportJSON string                `json:"context_report_json,omitempty"`
	StepDetail        []AgentRunStepRespDTO `json:"step_detail"`
}
