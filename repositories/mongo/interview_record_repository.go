package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const interviewRecordsCollection = "interview_records"

// CreateInterviewRecord 创建面试记录
func CreateInterviewRecord(ctx context.Context, record *models.InterviewRecord) error {
	collection, err := GetCollection(interviewRecordsCollection)
	if err != nil {
		return err
	}
	record.CreatedAt = time.Now()
	_, err = collection.InsertOne(ctx, record)
	return err
}

// PageInterviewRecords 分页查询面试记录（可按 sessionID 过滤）
func PageInterviewRecords(ctx context.Context, userID, sessionID string, page, size int) ([]models.InterviewRecord, int64, error) {
	collection, err := GetCollection(interviewRecordsCollection)
	if err != nil {
		return nil, 0, err
	}

	page, size = normalizePage(page, size)
	filter := bson.M{"user_id": userID}
	if sessionID != "" {
		filter["session_id"] = sessionID
	}

	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * size)).
		SetLimit(int64(size))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var records []models.InterviewRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// FindInterviewRecordBySessionID 按 sessionID 查询面试记录（取第一条）
func FindInterviewRecordBySessionID(ctx context.Context, sessionID, userID string) (*models.InterviewRecord, error) {
	collection, err := GetCollection(interviewRecordsCollection)
	if err != nil {
		return nil, err
	}
	var record models.InterviewRecord
	err = collection.FindOne(ctx, bson.M{"session_id": sessionID, "user_id": userID}).Decode(&record)
	if err != nil {
		return nil, err
	}
	return &record, nil
}
