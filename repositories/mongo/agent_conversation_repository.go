package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const agentConversationsCollection = "agent_conversations"

// CreateAgentConversation 创建 Agent 会话（upsert by sessionID）
func CreateAgentConversation(ctx context.Context, conv *models.AgentConversation) error {
	collection, err := GetCollection(agentConversationsCollection)
	if err != nil {
		return err
	}
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now
	update := bson.M{
		"$set": bson.M{
			"user_id":     conv.UserID,
			"agent_id":    conv.AgentID,
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

// PageAgentConversations 分页查询用户的 Agent 会话列表
func PageAgentConversations(ctx context.Context, userID string, offset, limit int) ([]models.AgentConversation, int64, error) {
	collection, err := GetCollection(agentConversationsCollection)
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

	var conversations []models.AgentConversation
	if err := cursor.All(ctx, &conversations); err != nil {
		return nil, 0, err
	}
	return conversations, total, nil
}

// EndAgentConversation 结束 Agent 会话（status=0）
func EndAgentConversation(ctx context.Context, sessionID, userID string) error {
	collection, err := GetCollection(agentConversationsCollection)
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

// GetAgentConversationBySessionId 按会话 ID 和用户 ID 查询 Agent 会话
func GetAgentConversationBySessionId(ctx context.Context, sessionID, userID string) (*models.AgentConversation, error) {
	collection, err := GetCollection(agentConversationsCollection)
	if err != nil {
		return nil, err
	}
	var conv models.AgentConversation
	err = collection.FindOne(ctx, bson.M{"_id": sessionID, "user_id": userID}).Decode(&conv)
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateAgentConversationMessageCount 更新会话消息计数
func UpdateAgentConversationMessageCount(ctx context.Context, sessionID string, messageCnt int) error {
	collection, err := GetCollection(agentConversationsCollection)
	if err != nil {
		return err
	}
	update := bson.M{
		"$set": bson.M{
			"message_cnt": messageCnt,
			"updated_at":  time.Now(),
		},
	}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": sessionID}, update)
	return err
}
