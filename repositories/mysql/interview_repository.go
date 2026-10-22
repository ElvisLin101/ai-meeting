package mysql

import (
	"ai-meeting/models"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ============================================================
// 面试会话 / 记录 / 题目 MySQL 仓储
// ============================================================

// CreateInterviewSession 创建面试会话（按 session_id 幂等 upsert）
func CreateInterviewSession(ctx context.Context, session *models.InterviewSession) error {
	now := time.Now()
	row := &models.InterviewSessionRow{
		SessionID:  session.SessionID,
		UserID:     session.UserID,
		Status:     session.Status,
		ResumePath: session.ResumePath,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "status", "resume_path", "updated_at"}),
	}).Create(row).Error
}

// EndInterviewSession 结束面试会话
func EndInterviewSession(ctx context.Context, sessionID, userID string) error {
	return DB.WithContext(ctx).Model(&models.InterviewSessionRow{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Updates(map[string]interface{}{"status": 0, "updated_at": time.Now()}).Error
}

// DeleteInterviewSession 删除面试会话（补偿用）
func DeleteInterviewSession(ctx context.Context, sessionID string) error {
	return DB.WithContext(ctx).Where("session_id = ?", sessionID).
		Delete(&models.InterviewSessionRow{}).Error
}

// UpdateResumePath 更新简历路径
func UpdateResumePath(ctx context.Context, sessionID, userID, resumePath string) error {
	return DB.WithContext(ctx).Model(&models.InterviewSessionRow{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Updates(map[string]interface{}{"resume_path": resumePath, "updated_at": time.Now()}).Error
}

// FindInterviewSession 查询面试会话（不存在返回 nil, nil）
func FindInterviewSession(ctx context.Context, sessionID, userID string) (*models.InterviewSession, error) {
	var row models.InterviewSessionRow
	err := DB.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &models.InterviewSession{
		SessionID: row.SessionID, UserID: row.UserID, Status: row.Status,
		ResumePath: row.ResumePath, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// CreateInterviewRecord 创建面试记录
func CreateInterviewRecord(ctx context.Context, record *models.InterviewRecord) error {
	row := &models.InterviewRecordRow{
		SessionID: record.SessionID, UserID: record.UserID,
		QuestionNum: record.QuestionNum, Question: record.Question,
		Answer: record.Answer, Score: record.Score,
		Suggestions: record.Suggestions, CreatedAt: time.Now(),
	}
	return DB.WithContext(ctx).Create(row).Error
}

// PageInterviewRecords 分页查询面试记录
func PageInterviewRecords(ctx context.Context, userID, sessionID string, page, size int) ([]models.InterviewRecord, int64, error) {
	page, size = normalizePage(page, size)
	query := DB.WithContext(ctx).Model(&models.InterviewRecordRow{}).Where("user_id = ?", userID)
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.InterviewRecordRow
	if err := query.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	records := make([]models.InterviewRecord, 0, len(rows))
	for _, r := range rows {
		records = append(records, models.InterviewRecord{
			SessionID: r.SessionID, UserID: r.UserID, QuestionNum: r.QuestionNum,
			Question: r.Question, Answer: r.Answer, Score: r.Score,
			Suggestions: r.Suggestions, CreatedAt: r.CreatedAt,
		})
	}
	return records, total, nil
}

// FindInterviewRecordBySessionID 按会话查询面试记录
func FindInterviewRecordBySessionID(ctx context.Context, sessionID, userID string) (*models.InterviewRecord, error) {
	var row models.InterviewRecordRow
	err := DB.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("id DESC").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &models.InterviewRecord{
		SessionID: row.SessionID, UserID: row.UserID, QuestionNum: row.QuestionNum,
		Question: row.Question, Answer: row.Answer, Score: row.Score,
		Suggestions: row.Suggestions, CreatedAt: row.CreatedAt,
	}, nil
}

// CreateInterviewQuestion 创建面试题目
func CreateInterviewQuestion(ctx context.Context, q *models.InterviewQuestion) error {
	row := &models.InterviewQuestionRow{
		SessionID: q.SessionID, Question: q.Question, QuestionNo: q.QuestionNo,
		OrderNum: q.OrderNum, Status: q.Status, CreatedAt: time.Now(),
	}
	return DB.WithContext(ctx).Create(row).Error
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return page, size
}
