package agent

// BusinessAgentScene 业务智能体场景枚举
// 每个场景对应一个讯飞星辰工作流，通过场景名称解析到具体的 AgentProperties
type BusinessAgentScene int

const (
	// SceneGeneralAgentChat 通用智能体聊天
	SceneGeneralAgentChat BusinessAgentScene = iota
	// SceneInterviewQuestionExtraction 面试出题
	SceneInterviewQuestionExtraction
	// SceneInterviewAnswerEvaluation 面试评分
	SceneInterviewAnswerEvaluation
	// SceneInterviewDemeanor 神态评估
	SceneInterviewDemeanor
	// SceneInterviewQuestionAsking 面试追问
	SceneInterviewQuestionAsking
)

// agentSceneConfig 场景配置：默认名称 + 别名列表
type agentSceneConfig struct {
	code                string
	defaultAgentName    string
	candidateAgentNames []string
}

var agentSceneConfigs = map[BusinessAgentScene]agentSceneConfig{
	SceneGeneralAgentChat: {
		code:                "general-agent-chat",
		defaultAgentName:    "通用智能体",
		candidateAgentNames: []string{"通用智能体"},
	},
	SceneInterviewQuestionExtraction: {
		code:                "interview-question-extraction",
		defaultAgentName:    "面试出题官",
		candidateAgentNames: []string{"面试出题官", "面试题出题官"},
	},
	SceneInterviewAnswerEvaluation: {
		code:                "interview-answer-evaluation",
		defaultAgentName:    "用户答案评分官",
		candidateAgentNames: []string{"用户答案评分官", "面试答案评分官"},
	},
	SceneInterviewDemeanor: {
		code:                "interview-demeanor",
		defaultAgentName:    "神态分析官",
		candidateAgentNames: []string{"神态分析官", "神态评分面试官", "表情分析面试官"},
	},
	SceneInterviewQuestionAsking: {
		code:                "interview-question-asking",
		defaultAgentName:    "面试提问官",
		candidateAgentNames: []string{"面试提问官"},
	},
}

// GetCandidateAgentNames 获取场景的候选 Agent 名称列表（含默认名 + 别名）
func (s BusinessAgentScene) GetCandidateAgentNames() []string {
	cfg, ok := agentSceneConfigs[s]
	if !ok {
		return nil
	}
	return cfg.candidateAgentNames
}

// GetCode 获取场景代码
func (s BusinessAgentScene) GetCode() string {
	cfg, ok := agentSceneConfigs[s]
	if !ok {
		return ""
	}
	return cfg.code
}
