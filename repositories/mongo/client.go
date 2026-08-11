package mongo

import (
	"ai-meeting/config"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	Client              *drivermongo.Client
	ErrMongoUnavailable = errors.New("mongodb is not initialized")
)

func InitMongoDB() error {
	cfg := config.AppConfig.MongoDB

	uri := fmt.Sprintf("mongodb://%s:%d", cfg.Host, cfg.Port)
	if cfg.Username != "" && cfg.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	Client, err = drivermongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}

	if err := Client.Ping(ctx, nil); err != nil {
		return err
	}

	logrus.Info("MongoDB connection established")

	// 建索引(幂等, 失败仅告警不阻断启动)
	ensureIndexes()
	return nil
}

// ensureIndexes 为高频查询集合建立复合索引, 覆盖会话/消息/归档的
// "过滤字段 + 排序字段" 查询模式, 避免数据量上来后全表扫 + 内存排序。
// 建索引失败只 warn——历史脏数据或权限问题不应阻塞服务拉起。
func ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	type indexSpec struct {
		collection string
		keys       bson.D
		unique     bool
	}
	specs := []indexSpec{
		{aiMessagesCollection, bson.D{{Key: "session_id", Value: 1}, {Key: "user_id", Value: 1}, {Key: "sequence", Value: -1}}, false},
		{agentMessagesCollection, bson.D{{Key: "session_id", Value: 1}, {Key: "user_id", Value: 1}, {Key: "sequence", Value: 1}}, false},
		{aiConversationsCollection, bson.D{{Key: "user_id", Value: 1}, {Key: "updated_at", Value: -1}}, false},
		{agentConversationsCollection, bson.D{{Key: "user_id", Value: 1}, {Key: "updated_at", Value: -1}}, false},
		{interviewRecordsCollection, bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, false},
		{interviewQuestionsCollection, bson.D{{Key: "session_id", Value: 1}}, false},
		{interviewSessionsCollection, bson.D{{Key: "user_id", Value: 1}}, false},
		{turnArchiveCollection, bson.D{{Key: "session_id", Value: 1}, {Key: "seq", Value: 1}}, false},
		{turnArchiveCollection, bson.D{{Key: "session_id", Value: 1}, {Key: "request_id", Value: 1}}, false},
	}

	for _, s := range specs {
		col, err := GetCollection(s.collection)
		if err != nil {
			logrus.Warnf("Skip index on %s: %v", s.collection, err)
			continue
		}
		opts := options.Index().SetUnique(s.unique)
		if _, err := col.Indexes().CreateOne(ctx, drivermongo.IndexModel{Keys: s.keys, Options: opts}); err != nil {
			logrus.Warnf("Failed to create index on %s %v: %v", s.collection, s.keys, err)
		}
	}
	logrus.Info("MongoDB indexes ensured")
}

func GetCollection(name string) (*drivermongo.Collection, error) {
	if Client == nil {
		return nil, ErrMongoUnavailable
	}
	return Client.Database(config.AppConfig.MongoDB.DBName).Collection(name), nil
}
