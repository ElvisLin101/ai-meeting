package mysql

import "ai-meeting/models"

func CreateInterviewRecord(record *models.InterviewRecord) error {
	return DB.Create(record).Error
}

func PageInterviewRecords(userID string, sessionID string, page, size int) ([]models.InterviewRecord, int64, error) {
	var records []models.InterviewRecord
	var total int64
	query := DB.Model(&models.InterviewRecord{}).Where("user_id = ?", userID)
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}
	offset := (page - 1) * size
	result := query.Count(&total).Order("created_at DESC").Offset(offset).Limit(size).Find(&records)
	return records, total, result.Error
}

func FindInterviewRecordBySessionID(sessionID, userID string) (*models.InterviewRecord, error) {
	var record models.InterviewRecord
	result := DB.Where("session_id = ? AND user_id = ?", sessionID, userID).First(&record)
	return &record, result.Error
}
