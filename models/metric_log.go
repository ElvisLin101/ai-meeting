package models

import "time"

// ============================================================
// MetricLog 统一指标日志
// 所有模块的量化指标异步写入此表, 不阻塞主流程
// 通过 module + event 区分不同指标
// ============================================================

type MetricLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Module      string    `gorm:"size:50;index;not null" json:"module"`            // ai_call/singleflight/state_machine/snapshot/repair/idempotency/lock
	Event       string    `gorm:"size:50;index;not null" json:"event"`             // 事件类型(如 eval_success/leader_elected/cas_retry)
	SessionID   string    `gorm:"size:64;index" json:"session_id"`                 // 关联的会话ID(可选)
	Success     bool      `gorm:"default:false" json:"success"`                    // 是否成功
	ErrorType   string    `gorm:"size:100" json:"error_type"`                      // 失败类型(parse_fail/schema_fail/ai_call_fail/timeout)
	IsRetry     bool      `gorm:"default:false" json:"is_retry"`                   // 是否为重试
	DurationMs  int64     `gorm:"default:0" json:"duration_ms"`                    // 耗时(毫秒)
	Extra       string    `gorm:"type:text" json:"extra"`                          // 额外信息(JSON, 如 stage/question_number)
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

func (MetricLog) TableName() string {
	return "metric_logs"
}
