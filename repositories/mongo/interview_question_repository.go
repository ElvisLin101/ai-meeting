package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

const interviewQuestionsCollection = "interview_questions"

// SaveInterviewQuestion 保存面试题目
func SaveInterviewQuestion(ctx context.Context, q *models.InterviewQuestion) error {
	collection, err := GetCollection(interviewQuestionsCollection)
	if err != nil {
		return err
	}
	q.CreatedAt = time.Now()
	_, err = collection.InsertOne(ctx, q)
	return err
}

// FindInterviewQuestionsBySessionID 按 sessionID 查询所有面试题目
func FindInterviewQuestionsBySessionID(ctx context.Context, sessionID string) ([]models.InterviewQuestion, error) {
	collection, err := GetCollection(interviewQuestionsCollection)
	if err != nil {
		return nil, err
	}
	cursor, err := collection.Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var questions []models.InterviewQuestion
	if err := cursor.All(ctx, &questions); err != nil {
		return nil, err
	}
	return questions, nil
}
