package mongo

import (
	"ai-meeting/models"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const compressedContextsCollection = "compressed_contexts"

type CompressedContextUpsert struct {
	ID                string
	SessionID         string
	MemoryScope       string
	CompressedContent string
	Index             int
	TotalLength       int
	MessageCount      int
}

func UpsertCompressedContext(ctx context.Context, data CompressedContextUpsert) error {
	collection, err := GetCollection(compressedContextsCollection)
	if err != nil {
		return err
	}

	now := time.Now()
	setFields := bson.M{
		"session_id":         data.SessionID,
		"compressed_content": data.CompressedContent,
		"index":              data.Index,
		"total_token_count":  int64(data.TotalLength),
		"message_count":      data.MessageCount,
		"updated_at":         now,
	}
	if data.MemoryScope != "" {
		setFields["memory_scope"] = data.MemoryScope
	}

	update := bson.M{
		"$set": setFields,
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": data.ID}, update, opts)
	return err
}

func FindCompressedContextByID(ctx context.Context, id string) (*models.CompressedContext, error) {
	collection, err := GetCollection(compressedContextsCollection)
	if err != nil {
		return nil, err
	}

	var result models.CompressedContext
	err = collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err == drivermongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func FindCompressedContextBySessionID(ctx context.Context, sessionID string) (*models.CompressedContext, error) {
	collection, err := GetCollection(compressedContextsCollection)
	if err != nil {
		return nil, err
	}

	var result models.CompressedContext
	err = collection.FindOne(ctx, bson.M{"session_id": sessionID}).Decode(&result)
	if err == drivermongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func DeleteCompressedContextByID(ctx context.Context, id string) error {
	collection, err := GetCollection(compressedContextsCollection)
	if err != nil {
		return err
	}

	_, err = collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
