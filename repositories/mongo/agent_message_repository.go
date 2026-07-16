package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const agentMessagesCollection = "agent_messages"

func ListAgentMessagesAsc(ctx context.Context, sessionID, userID string) ([]models.AgentMessage, error) {
	return listAgentMessages(ctx, sessionID, userID, 1, 0)
}

func ListAgentMessagesDesc(ctx context.Context, sessionID, userID string) ([]models.AgentMessage, error) {
	return listAgentMessages(ctx, sessionID, userID, -1, 0)
}

func ListAgentMessagesAfterSequenceDesc(ctx context.Context, sessionID, userID string, sequence int) ([]models.AgentMessage, error) {
	return listAgentMessages(ctx, sessionID, userID, -1, sequence)
}

func listAgentMessages(ctx context.Context, sessionID, userID string, sortDirection int, afterSequence int) ([]models.AgentMessage, error) {
	collection, err := GetCollection(agentMessagesCollection)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"session_id": sessionID, "user_id": userID}
	if afterSequence > 0 {
		filter["sequence"] = bson.M{"$gt": afterSequence}
	}

	opts := options.Find().SetSort(bson.D{{Key: "sequence", Value: sortDirection}})
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []models.AgentMessage
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func PageAgentMessages(ctx context.Context, sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	collection, err := GetCollection(agentMessagesCollection)
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
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "sequence", Value: -1}}).
		SetSkip(int64((page - 1) * size)).
		SetLimit(int64(size))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var messages []models.AgentMessage
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

func SaveAgentMessage(ctx context.Context, sessionID, userID, role, content string) error {
	collection, err := GetCollection(agentMessagesCollection)
	if err != nil {
		return err
	}

	maxSeq, err := nextAgentMessageSequence(ctx, collection, sessionID, userID)
	if err != nil {
		return err
	}

	message := models.AgentMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      role,
		Content:   content,
		Sequence:  maxSeq,
		CreatedAt: time.Now(),
	}
	_, err = collection.InsertOne(ctx, message)
	return err
}

func nextAgentMessageSequence(ctx context.Context, collection *drivermongo.Collection, sessionID, userID string) (int, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "sequence", Value: -1}})
	filter := bson.M{"session_id": sessionID, "user_id": userID}

	var latest models.AgentMessage
	err := collection.FindOne(ctx, filter, opts).Decode(&latest)
	if err == drivermongo.ErrNoDocuments {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return latest.Sequence + 1, nil
}

// SaveAgentMessageWithDetail 保存消息（含 responseTime 和 errorMessage，用于 assistant 消息）
func SaveAgentMessageWithDetail(ctx context.Context, sessionID, userID, role, content string, responseTime int64, errorMessage string) (int, error) {
	collection, err := GetCollection(agentMessagesCollection)
	if err != nil {
		return 0, err
	}

	maxSeq, err := nextAgentMessageSequence(ctx, collection, sessionID, userID)
	if err != nil {
		return 0, err
	}

	message := models.AgentMessage{
		SessionID:    sessionID,
		UserID:       userID,
		Role:         role,
		Content:      content,
		Sequence:     maxSeq,
		ResponseTime: responseTime,
		ErrorMessage: errorMessage,
		CreatedAt:    time.Now(),
	}
	_, err = collection.InsertOne(ctx, message)
	if err != nil {
		return 0, err
	}
	return maxSeq, nil
}
