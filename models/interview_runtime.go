package models

import "time"

// ============================================================
// 面试运行态模型
//
// 状态机阶段: INIT → ASKING → EVALUATING → FOLLOW_UP → ... → COMPLETED
// 详见 docs/agent-knowledge/references/interview-runtime-governance.md
// ============================================================

// InterviewFlowStatus 面试流程阶段
type InterviewFlowStatus string

const (
	FlowInit       InterviewFlowStatus = "INIT"       // 初始化态, 刚创建 flow, 几乎不驻留
	FlowAsking     InterviewFlowStatus = "ASKING"     // 正在询问主问题, 等待用户作答
	FlowEvaluating InterviewFlowStatus = "EVALUATING" // 正在评分(AI 评分中)
	FlowFollowUp   InterviewFlowStatus = "FOLLOW_UP"  // 正在追问
	FlowCompleted  InterviewFlowStatus = "COMPLETED"  // 面试结束, 终态, 不可再转移
)

// InterviewFlowState 面试流程状态
// 存 Redis Hash (interview:flow:session:{sid}) + Mongo 热快照
type InterviewFlowState struct {
	Status              InterviewFlowStatus `bson:"status" json:"status"`
	CurrentIndex        int                 `bson:"current_index" json:"current_index"`               // 主问题下标(0-based)
	CurrentQuestionNumber string           `bson:"current_question_number" json:"current_question_number"` // "1" 主问题, "1-F1" 追问
	TotalQuestions      int                 `bson:"total_questions" json:"total_questions"`           // 主问题总数
	FollowUpCount       int                 `bson:"follow_up_count" json:"follow_up_count"`           // 当前主问题已追问轮次
	MaxFollowUp         int                 `bson:"max_follow_up" json:"max_follow_up"`               // 单主问题追问上限(默认2)
	Version             int                 `bson:"version" json:"version"`                           // CAS 乐观锁版本号
}

// IsCompleted 面试是否已结束
func (s InterviewFlowState) IsCompleted() bool {
	return s.Status == FlowCompleted
}

// IsOutOfRange 当前题号是否越界(到末题之后)
func (s InterviewFlowState) IsOutOfRange() bool {
	return s.CurrentIndex >= s.TotalQuestions
}

// InterviewTurnLog 单轮问答日志
// 存 Redis List (interview:turns:session:{sid}) + Mongo TurnArchive
type InterviewTurnLog struct {
	Timestamp          time.Time `bson:"timestamp" json:"timestamp"`
	RequestID          string    `bson:"request_id" json:"request_id"`                      // 幂等键
	QuestionNumber     string    `bson:"question_number" json:"question_number"`            // 题号(主"1"/追问"1-F1")
	QuestionContent    string    `bson:"question_content" json:"question_content"`
	AnswerContent      string    `bson:"answer_content" json:"answer_content"`              // 截断到1000字符
	Score              int       `bson:"score" json:"score"`                                // 本轮得分[0,100]
	TotalScore         int       `bson:"total_score" json:"total_score"`                    // 累计平均分(本轮后)
	Feedback           string    `bson:"feedback" json:"feedback"`                          // AI 反馈
	FollowUpNeeded     bool      `bson:"follow_up_needed" json:"follow_up_needed"`
	IsFollowUp         bool      `bson:"is_follow_up" json:"is_follow_up"`                  // 本轮是否为追问题
	FollowUpCount      int       `bson:"follow_up_count" json:"follow_up_count"`            // 当前追问轮次
	NextQuestionNumber string    `bson:"next_question_number" json:"next_question_number"`  // 下一题号
	NextQuestion       string    `bson:"next_question" json:"next_question"`                // 下一题题面
	Finished           bool      `bson:"finished" json:"finished"`                          // 是否面试结束
}

// ============================================================
// Mongo 持久化实体（热冷快照 + 轮次归档）
// ============================================================

// InterviewRuntimeHotSnapshot 热快照
// 高频变化的流程态, CAS 乐观锁更新
// Mongo 集合: interview_session_runtime_hot_snapshot
type InterviewRuntimeHotSnapshot struct {
	SessionID                string              `bson:"_id" json:"session_id"`
	UserID                   string              `bson:"user_id" json:"user_id"`
	SessionStatus            int                 `bson:"session_status" json:"session_status"`
	SnapshotVersion          int64               `bson:"snapshot_version" json:"snapshot_version"`           // CAS 版本号
	SnapshotLevel            string              `bson:"snapshot_level" json:"snapshot_level"`               // DRAFT/QUESTION_READY/ACTIVE/FINALIZED
	Flow                     InterviewFlowState  `bson:"flow" json:"flow"`
	ScoreSum                 int                 `bson:"score_sum" json:"score_sum"`
	ScoreCount               int                 `bson:"score_count" json:"score_count"`
	FollowUpQuestions        map[string]string   `bson:"follow_up_questions" json:"follow_up_questions"`
	RecentTurns              []InterviewTurnLog  `bson:"recent_turns" json:"recent_turns"`                   // 最近20轮窗口
	RecentTurnCount          int                 `bson:"recent_turn_count" json:"recent_turn_count"`
	ArchiveWatermark         int64               `bson:"archive_watermark" json:"archive_watermark"`         // 已归档水位线
	LastTurnSeq              int64               `bson:"last_turn_seq" json:"last_turn_seq"`                 // 单调保护
	LastAppliedRequestID     string              `bson:"last_applied_request_id" json:"last_applied_request_id"`
	LastMutationID           string              `bson:"last_mutation_id" json:"last_mutation_id"`           // = requestId, 幂等短路
	LastCommittedQuestionNumber string           `bson:"last_committed_question_number" json:"last_committed_question_number"`
	CreatedAt                time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt                time.Time           `bson:"updated_at" json:"updated_at"`
}

// InterviewRuntimeColdSnapshot 冷快照
// 低频变化的材料数据, 无 CAS
// Mongo 集合: interview_session_runtime_cold_snapshot
type InterviewRuntimeColdSnapshot struct {
	SessionID       string                 `bson:"_id" json:"session_id"`
	UserID          string                 `bson:"user_id" json:"user_id"`
	MaterialVersion int64                  `bson:"material_version" json:"material_version"`
	InterviewType   string                 `bson:"interview_type" json:"interview_type"`
	Direction       string                 `bson:"direction" json:"direction"`
	Questions       map[string]string      `bson:"questions" json:"questions"`
	Suggestions     map[string]string      `bson:"suggestions" json:"suggestions"`
	ResumeContext   map[string]interface{} `bson:"resume_context" json:"resume_context"`
	ResumeScore     int                    `bson:"resume_score" json:"resume_score"`
	CreatedAt       time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time              `bson:"updated_at" json:"updated_at"`
}

// InterviewSessionTurnArchive 轮次归档
// 完整不可变的轮次历史, 按 seq 单调递增
// Mongo 集合: interview_session_turn_archive
type InterviewSessionTurnArchive struct {
	ID              string            `bson:"_id" json:"id"`
	SessionID       string            `bson:"session_id" json:"session_id"`    // 索引
	RequestID       string            `bson:"request_id" json:"request_id"`    // 索引, 幂等查重
	Seq             int64             `bson:"seq" json:"seq"`                  // 单调递增, 索引
	SnapshotVersion int64             `bson:"snapshot_version" json:"snapshot_version"`
	TurnPayload     InterviewTurnLog  `bson:"turn_payload" json:"turn_payload"`
	CreatedAt       time.Time         `bson:"created_at" json:"created_at"`
}

// 热快照窗口限制
const HotSnapshotRecentTurnLimit = 20
