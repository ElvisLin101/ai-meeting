package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const interviewSessionsCollection = "interview_sessions"

// CreateInterviewSession 创建面试会话（upsert by sessionID）
func CreateInterviewSession(ctx context.Context, session *models.InterviewSession) error {
	collection, err := GetCollection(interviewSessionsCollection)
	if err != nil {
		return err
	}
	now := time.Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	update := bson.M{
		"$set": bson.M{
			"user_id":     session.UserID,
			"status":      session.Status,
			"resume_path": session.ResumePath,
			"updated_at":  now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": session.SessionID}, update, opts)
	return err
}

// EndInterviewSession 结束面试会话（status=0）
func EndInterviewSession(ctx context.Context, sessionID, userID string) error {
	collection, err := GetCollection(interviewSessionsCollection)
	if err != nil {
		return err
	}
	update := bson.M{
		"$set": bson.M{
			"status":     0,
			"updated_at": time.Now(),
		},
	}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": sessionID, "user_id": userID}, update)
	return err
}
