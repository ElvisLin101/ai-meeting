package clients

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ============================================================
// AI 模型配置全局缓存
//
// 热点路径（每次 AI 调用）不再直接打 MySQL，改为读全局缓存:
//   - TTL 到期自动刷新
//   - 未命中(404)走短 TTL 负缓存，避免模型不存在时反复打库
//   - 属性增删改/启停时通过 InvalidateAiPropertyCache 主动失效，
//     保证后台配置改动秒级生效
// ============================================================

const (
	aiPropertyCacheTTL    = 60 * time.Second // 生效配置缓存时间
	aiPropertyNegativeTTL = 10 * time.Second // 未命中(404)缓存时间
)

type aiPropertyCacheEntry struct {
	prop      *models.AiProperties
	err       error
	expiresAt time.Time
}

var (
	aiPropertyCacheMu sync.Mutex
	aiPropertyCache   = make(map[uint]aiPropertyCacheEntry)
)

// GetEnabledAiProperty 读取启用的 AI 配置（带全局缓存）
func GetEnabledAiProperty(aiID uint) (*models.AiProperties, error) {
	now := time.Now()

	aiPropertyCacheMu.Lock()
	if e, ok := aiPropertyCache[aiID]; ok && now.Before(e.expiresAt) {
		aiPropertyCacheMu.Unlock()
		return e.prop, e.err
	}
	aiPropertyCacheMu.Unlock()

	prop, err := mysqlrepo.FindEnabledAiProperty(aiID)

	ttl := aiPropertyCacheTTL
	if err != nil {
		ttl = aiPropertyNegativeTTL
	}
	aiPropertyCacheMu.Lock()
	aiPropertyCache[aiID] = aiPropertyCacheEntry{prop: prop, err: err, expiresAt: now.Add(ttl)}
	aiPropertyCacheMu.Unlock()

	if err != nil {
		logrus.Warnf("FindEnabledAiProperty(aiID=%d) failed: %v", aiID, err)
	}
	return prop, err
}

// InvalidateAiPropertyCache 清除 AI 配置缓存（属性增删改/启停后调用）
func InvalidateAiPropertyCache() {
	aiPropertyCacheMu.Lock()
	aiPropertyCache = make(map[uint]aiPropertyCacheEntry)
	aiPropertyCacheMu.Unlock()
	logrus.Debug("AI property cache invalidated")
}
