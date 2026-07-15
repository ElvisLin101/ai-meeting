package mysql

import "ai-meeting/models"

func CreateInterviewSession(session *models.InterviewSession) error {
	return DB.Create(session).Error
}

func EndInterviewSession(sessionID, userID string) error {
	result := DB.Model(&models.InterviewSession{}).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Update("status", 0)
	return result.Error
}
