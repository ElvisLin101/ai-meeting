package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const aiConversationsCollection = "ai_conversations"

// CreateAiConversation 创建 AI 会话（upsert by sessionID）
func CreateAiConversation(ctx context.Context, conv *models.AiConversation) error {
	collection, err := GetCollection(aiConversationsCollection)
	if err != nil {
		return err
	}
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now
	update := bson.M{
		"$set": bson.M{
			"user_id":     conv.UserID,
			"ai_id":       conv.AiID,
			"title":       conv.Title,
			"status":      conv.Status,
			"message_cnt": conv.MessageCnt,
			"updated_at":  now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": conv.SessionID}, update, opts)
	return err
}

// PageAiConversations 分页查询用户的 AI 会话列表
func PageAiConversations(ctx context.Context, userID string, offset, limit int) ([]models.AiConversation, int64, error) {
	collection, err := GetCollection(aiConversationsCollection)
	if err != nil {
		return nil, 0, err
	}

	filter := bson.M{"user_id": userID}
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(int64(offset)).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var conversations []models.AiConversation
	if err := cursor.All(ctx, &conversations); err != nil {
		return nil, 0, err
	}
	return conversations, total, nil
}

// UpdateAiConversation 按 sessionID 更新 AI 会话字段
func UpdateAiConversation(ctx context.Context, sessionID, userID string, updates map[string]interface{}) error {
	collection, err := GetCollection(aiConversationsCollection)
	if err != nil {
		return err
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["updated_at"] = time.Now()
	update := bson.M{"$set": updates}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": sessionID, "user_id": userID}, update)
	return err
}

// EndAiConversation 结束 AI 会话（status=0）
func EndAiConversation(ctx context.Context, sessionID, userID string) error {
	collection, err := GetCollection(aiConversationsCollection)
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

// DeleteAiConversation 删除 AI 会话
func DeleteAiConversation(ctx context.Context, sessionID, userID string) error {
	collection, err := GetCollection(aiConversationsCollection)
	if err != nil {
		return err
	}
	_, err = collection.DeleteOne(ctx, bson.M{"_id": sessionID, "user_id": userID})
	return err
}

// FindAiConversationBySessionID 按 sessionID 查询 AI 会话
func FindAiConversationBySessionID(ctx context.Context, sessionID, userID string) (*models.AiConversation, error) {
	collection, err := GetCollection(aiConversationsCollection)
	if err != nil {
		return nil, err
	}
	var conv models.AiConversation
	err = collection.FindOne(ctx, bson.M{"_id": sessionID, "user_id": userID}).Decode(&conv)
	if err != nil {
		return nil, err
	}
	return &conv, nil
}
