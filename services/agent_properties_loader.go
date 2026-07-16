package services

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"sync"

	"github.com/sirupsen/logrus"
)

// AgentPropertiesLoader 智能体配置启动缓存
// 启动时全量加载 agent_properties 到内存，运行时先查缓存 miss 查库
type AgentPropertiesLoader struct {
	cache sync.Map // uint -> *models.AgentProperties
}

var agentPropertiesLoaderInstance *AgentPropertiesLoader
var agentPropertiesLoaderOnce sync.Once

func GetAgentPropertiesLoader() *AgentPropertiesLoader {
	agentPropertiesLoaderOnce.Do(func() {
		agentPropertiesLoaderInstance = &AgentPropertiesLoader{}
		agentPropertiesLoaderInstance.RefreshActiveAgents()
	})
	return agentPropertiesLoaderInstance
}

// RefreshActiveAgents 刷新缓存：全量加载启用的智能体配置
func (l *AgentPropertiesLoader) RefreshActiveAgents() {
	agents, err := mysqlrepo.ListActiveAgentProperties()
	if err != nil {
		logrus.Errorf("Failed to load agent properties: %v", err)
		return
	}

	l.cache.Range(func(key, value interface{}) bool {
		l.cache.Delete(key)
		return true
	})

	for i := range agents {
		l.cache.Store(agents[i].ID, &agents[i])
	}
	logrus.Infof("Loaded %d agent properties into startup cache", len(agents))
}

// GetByAgentID 按 ID 查询（先缓存 miss 查库）
func (l *AgentPropertiesLoader) GetByAgentID(agentID uint) *models.AgentProperties {
	if agentID == 0 {
		return nil
	}

	if cached, ok := l.cache.Load(agentID); ok {
		prop := cached.(*models.AgentProperties)
		if prop.IsEnabled {
			return prop
		}
		return nil
	}

	prop, err := mysqlrepo.FindAgentPropertiesByID(agentID)
	if err != nil || prop == nil {
		return nil
	}
	if !prop.IsEnabled {
		return nil
	}

	l.cache.Store(agentID, prop)
	return prop
}

// GetByAgentName 按名称查询（遍历缓存，miss 查库）
func (l *AgentPropertiesLoader) GetByAgentName(name string) *models.AgentProperties {
	if name == "" {
		return nil
	}

	var found *models.AgentProperties
	l.cache.Range(func(key, value interface{}) bool {
		prop := value.(*models.AgentProperties)
		if prop.Name == name && prop.IsEnabled {
			found = prop
			return false // 找到了，停止遍历
		}
		return true
	})

	if found != nil {
		return found
	}

	// 缓存 miss，查库
	prop, err := mysqlrepo.FindAgentPropertiesByName(name)
	if err != nil || prop == nil {
		return nil
	}
	if !prop.IsEnabled {
		return nil
	}

	l.cache.Store(prop.ID, prop)
	return prop
}

// ResolveRequired 根据业务场景解析智能体配置
// 按候选名称列表顺序匹配，第一个找到的返回
// 如果全部找不到返回 nil
func (l *AgentPropertiesLoader) ResolveRequired(scene BusinessAgentScene) *models.AgentProperties {
	candidates := scene.GetCandidateAgentNames()
	for _, name := range candidates {
		prop := l.GetByAgentName(name)
		if prop != nil {
			logrus.Infof("Resolved agent for scene %s: name=%s id=%d", scene.GetCode(), name, prop.ID)
			return prop
		}
	}

	logrus.Errorf("No agent found for scene %s, tried candidates: %v", scene.GetCode(), candidates)
	return nil
}
