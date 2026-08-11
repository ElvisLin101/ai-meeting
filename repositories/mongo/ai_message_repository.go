package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const aiMessagesCollection = "ai_messages"

func ListAiMessagesAsc(ctx context.Context, sessionID, userID string) ([]models.AiMessage, error) {
	return listAiMessages(ctx, sessionID, userID, 1, 0)
}

func ListAiMessagesDesc(ctx context.Context, sessionID, userID string) ([]models.AiMessage, error) {
	return listAiMessages(ctx, sessionID, userID, -1, 0)
}

func ListAiMessagesAfterSequenceDesc(ctx context.Context, sessionID, userID string, sequence int) ([]models.AiMessage, error) {
	return listAiMessages(ctx, sessionID, userID, -1, sequence)
}

func listAiMessages(ctx context.Context, sessionID, userID string, sortDirection int, afterSequence int) ([]models.AiMessage, error) {
	collection, err := GetCollection(aiMessagesCollection)
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

	var messages []models.AiMessage
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func PageAiMessages(ctx context.Context, sessionID string, page, size int, userID string) ([]models.AiMessage, int64, error) {
	collection, err := GetCollection(aiMessagesCollection)
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

	var messages []models.AiMessage
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

func SaveAiMessage(ctx context.Context, sessionID, userID, role, content string) (*models.AiMessage, error) {
	collection, err := GetCollection(aiMessagesCollection)
	if err != nil {
		return nil, err
	}

	sequence, err := nextAiMessageSequence(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}

	message := &models.AiMessage{
		MongoID:   primitive.NewObjectID(),
		SessionID: sessionID,
		UserID:    userID,
		Role:      role,
		Content:   content,
		Sequence:  sequence,
		CreatedAt: time.Now(),
	}
	if _, err := collection.InsertOne(ctx, message); err != nil {
		return nil, err
	}
	return message, nil
}

// nextAiMessageSequence 分配消息 seq(计数器原子递增, 替代"读 max+1"避免并发重复)
func nextAiMessageSequence(ctx context.Context, sessionID, userID string) (int, error) {
	seq, err := nextSeqFromCounter(ctx, "ai_msg_seq:"+sessionID+":"+userID)
	if err != nil {
		return 0, err
	}
	return int(seq), nil
}

func CountAiMessages(ctx context.Context, sessionID, userID string) (int, error) {
	collection, err := GetCollection(aiMessagesCollection)
	if err != nil {
		return 0, err
	}

	total, err := collection.CountDocuments(ctx, bson.M{"session_id": sessionID, "user_id": userID})
	if err != nil {
		return 0, err
	}
	return int(total), nil
}

func DeleteAiMessagesBySession(ctx context.Context, sessionID, userID string) error {
	collection, err := GetCollection(aiMessagesCollection)
	if err != nil {
		return err
	}

	_, err = collection.DeleteMany(ctx, bson.M{"session_id": sessionID, "user_id": userID})
	return err
}
