package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// counterCollection 原子自增计数器集合
// _id 形如 "turn_seq:{sessionID}" / "ai_msg_seq:{sessionID}:{userID}"
const counterCollection = "counters"

// nextSeqFromCounter 原子递增计数器并返回新值。
// 替代"读 max+1 再插入"的并发重复问题: findOneAndUpdate + $inc 在文档级原子,
// 并发 upsert 同一 _id 时可能触发 duplicate key, 重试兜底。
func nextSeqFromCounter(ctx context.Context, counterID string) (int64, error) {
	collection, err := GetCollection(counterCollection)
	if err != nil {
		return 0, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		var doc struct {
			Seq int64 `bson:"seq"`
		}
		err := collection.FindOneAndUpdate(ctx,
			bson.M{"_id": counterID},
			bson.M{"$inc": bson.M{"seq": 1}},
			options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
		).Decode(&doc)
		if err == nil {
			return doc.Seq, nil
		}
		if drivermongo.IsDuplicateKeyError(err) {
			continue // 并发 upsert 竞态, 重试
		}
		return 0, err
	}
	return 0, fmt.Errorf("counter increment failed after retries: %s", counterID)
}
