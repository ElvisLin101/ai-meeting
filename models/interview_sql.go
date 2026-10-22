package models

import "time"

// ============================================================
// 面试模块 MySQL 行模型
//
// 与 models/interview.go、models/interview_runtime.go 的领域模型区分:
// 领域模型保留业务语义(含 bson tag), 这里负责 SQL 存储,
// 复杂结构(flow/轮次/题目映射)统一序列化为 JSON 文本。
// ============================================================

// InterviewSessionRow 面试会话
type InterviewSessionRow struct {
	SessionID  string    `gorm:"primaryKey;size:64" json:"session_id"`
	UserID     string    `gorm:"size:128;index;not null" json:"user_id"`
	Status     int       `gorm:"index;default:1" json:"status"`
	ResumePath string    `gorm:"size:512" json:"resume_path"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 表名
func (InterviewSessionRow) TableName() string { return "interview_sessions" }

// InterviewRecordRow 面试记录/报告
type InterviewRecordRow struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SessionID   string    `gorm:"size:64;index;not null" json:"session_id"`
	UserID      string    `gorm:"size:128;index;not null" json:"user_id"`
	QuestionNum string    `gorm:"size:32" json:"question_num"`
	Question    string    `gorm:"type:longtext" json:"question"`
	Answer      string    `gorm:"type:longtext" json:"answer"`
	Score       int       `json:"score"`
	Suggestions string    `gorm:"type:longtext" json:"suggestions"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// TableName 表名
func (InterviewRecordRow) TableName() string { return "interview_records" }

// InterviewQuestionRow 面试题目
type InterviewQuestionRow struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"size:64;index;not null" json:"session_id"`
	Question   string    `gorm:"type:longtext" json:"question"`
	QuestionNo string    `gorm:"size:32" json:"question_no"`
	OrderNum   int       `json:"order_num"`
	Status     int       `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// TableName 表名
func (InterviewQuestionRow) TableName() string { return "interview_questions" }

// InterviewHotSnapshotRow 热快照（Flow 等结构以 JSON 存储）
type InterviewHotSnapshotRow struct {
	SessionID                   string    `gorm:"primaryKey;size:64" json:"session_id"`
	UserID                      string    `gorm:"size:128;index" json:"user_id"`
	SessionStatus               int       `json:"session_status"`
	SnapshotVersion             int64     `gorm:"not null;default:0" json:"snapshot_version"`
	SnapshotLevel               string    `gorm:"size:32" json:"snapshot_level"`
	FlowJSON                    string    `gorm:"type:longtext" json:"flow_json"`
	ScoreSum                    int       `json:"score_sum"`
	ScoreCount                  int       `json:"score_count"`
	FollowUpQuestionsJSON       string    `gorm:"type:longtext" json:"follow_up_questions_json"`
	RecentTurnsJSON             string    `gorm:"type:longtext" json:"recent_turns_json"`
	RecentTurnCount             int       `json:"recent_turn_count"`
	ArchiveWatermark            int64     `json:"archive_watermark"`
	LastTurnSeq                 int64     `json:"last_turn_seq"`
	LastAppliedRequestID        string    `gorm:"size:64" json:"last_applied_request_id"`
	LastMutationID              string    `gorm:"size:64;index" json:"last_mutation_id"`
	LastCommittedQuestionNumber string    `gorm:"size:32" json:"last_committed_question_number"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

// TableName 表名
func (InterviewHotSnapshotRow) TableName() string {
	return "interview_session_runtime_hot_snapshot"
}

// InterviewColdSnapshotRow 冷快照
type InterviewColdSnapshotRow struct {
	SessionID         string    `gorm:"primaryKey;size:64" json:"session_id"`
	UserID            string    `gorm:"size:128;index" json:"user_id"`
	MaterialVersion   int64     `json:"material_version"`
	InterviewType     string    `gorm:"size:64" json:"interview_type"`
	Direction         string    `gorm:"size:64" json:"direction"`
	QuestionsJSON     string    `gorm:"type:longtext" json:"questions_json"`
	SuggestionsJSON   string    `gorm:"type:longtext" json:"suggestions_json"`
	ResumeContextJSON string    `gorm:"type:longtext" json:"resume_context_json"`
	ResumeScore       int       `json:"resume_score"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TableName 表名
func (InterviewColdSnapshotRow) TableName() string {
	return "interview_session_runtime_cold_snapshot"
}

// InterviewTurnArchiveRow 轮次归档
type InterviewTurnArchiveRow struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	SessionID       string    `gorm:"size:64;not null;uniqueIndex:uk_turn_request,priority:1;uniqueIndex:uk_turn_seq,priority:1;index" json:"session_id"`
	RequestID       string    `gorm:"size:64;not null;uniqueIndex:uk_turn_request,priority:2" json:"request_id"`
	Seq             int64     `gorm:"not null;uniqueIndex:uk_turn_seq,priority:2" json:"seq"`
	SnapshotVersion int64     `json:"snapshot_version"`
	TurnPayloadJSON string    `gorm:"type:longtext" json:"turn_payload_json"`
	CreatedAt       time.Time `json:"created_at"`
}

// TableName 表名
func (InterviewTurnArchiveRow) TableName() string {
	return "interview_session_turn_archive"
}
