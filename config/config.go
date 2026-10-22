package config

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type AIConfig struct {
	Provider string         `mapstructure:"provider"`
	DeepSeek DeepSeekConfig `mapstructure:"deepseek"`
}

type DeepSeekConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Endpoint  string `mapstructure:"endpoint"`
	Model     string `mapstructure:"model"`
	APIKey    string `mapstructure:"api_key"`
	APISecret string `mapstructure:"api_secret"`
}

// AgentConfig Agent 运行时配置
type AgentConfig struct {
	MaxSteps            int           `mapstructure:"max_steps"`             // 单次 run 最大迭代步数
	ToolTimeoutSec      int           `mapstructure:"tool_timeout"`          // 单个工具执行超时(秒)
	TotalTimeoutSec     int           `mapstructure:"total_timeout"`         // 单次 run 总超时(秒)
	MaxToolResultBytes  int           `mapstructure:"max_tool_result_bytes"` // 工具结果截断上限
	MaxParallelTools    int           `mapstructure:"max_parallel_tools"`    // 单步内并行工具数上限
	SandboxRoot         string        `mapstructure:"sandbox_root"`          // 文件工具沙箱根目录
	ContextBudget       int           `mapstructure:"context_budget"`        // Deprecated: 旧字节预算，仅兼容旧配置
	ProfileExtractEvery int           `mapstructure:"profile_extract_every"` // 每 N 步尝试提炼画像
	Search              SearchConfig  `mapstructure:"search"`
	Plan                PlanConfig    `mapstructure:"plan"`
	Context             ContextConfig `mapstructure:"context"`
}

// ContextConfig 上下文 token 预算配置
type ContextConfig struct {
	ModelWindowTokens       int `mapstructure:"model_window_tokens"`       // 模型上下文窗口
	MaxOutputTokens         int `mapstructure:"max_output_tokens"`         // 输出预留
	SoftLimitTokens         int `mapstructure:"soft_limit_tokens"`         // 日常目标预算（硬上限仍看窗口）
	SafetyMarginTokens      int `mapstructure:"safety_margin_tokens"`      // 安全余量
	ReserveTokens           int `mapstructure:"reserve_tokens"`            // 工具 schema + query + 协议预留
	IdentityMaxTokens       int `mapstructure:"identity_max_tokens"`       // identity 上限
	ProfileMaxTokens        int `mapstructure:"profile_max_tokens"`        // 长期画像上限
	SummaryMaxTokens        int `mapstructure:"summary_max_tokens"`        // 中期摘要上限
	RecentMaxTokens         int `mapstructure:"recent_max_tokens"`         // 短期消息上限
	CompressThresholdTokens int `mapstructure:"compress_threshold_tokens"` // 触发压缩的未压缩消息 token 阈值
}

// PlanConfig Plan-and-Execute 编排配置
type PlanConfig struct {
	Enabled          bool   `mapstructure:"enabled"`            // 是否启用 plan 编排
	MaxTasks         int    `mapstructure:"max_tasks"`          // 单次计划最大任务数
	MaxParallel      int    `mapstructure:"max_parallel"`       // 计划内最大并行任务数
	TaskTimeoutSec   int    `mapstructure:"task_timeout"`       // 单个任务超时(秒)
	TotalTimeoutSec  int    `mapstructure:"total_timeout"`      // 计划总超时(秒)
	MaxStepsPerTask  int    `mapstructure:"max_steps_per_task"` // agent 子任务最大 ReAct 步数
	LongQueryRunes   int    `mapstructure:"long_query_runes"`   // 超过该长度视为复杂查询
	ReplanEnabled    bool   `mapstructure:"replan_enabled"`     // 任务失败/hint 时是否允许 replan
	MaxReplans       int    `mapstructure:"max_replans"`        // 单次 run 最大 replan 次数
	RetryMaxAttempts int    `mapstructure:"retry_max_attempts"` // 任务最大尝试次数（含首次）
	RetryBackoffMs   int    `mapstructure:"retry_backoff_ms"`   // 重试基础退避（毫秒）
	RouteMode        string `mapstructure:"mode"`               // auto | always | never
	RouteMinScore    int    `mapstructure:"min_score"`          // 进入 plan 的最低规则得分
	FastPathRunes    int    `mapstructure:"fast_path_runes"`    // 短查询快速通道阈值
}

// SearchConfig 网页搜索配置
type SearchConfig struct {
	Provider   string `mapstructure:"provider"` // tavily/serper/bing
	APIKey     string `mapstructure:"api_key"`
	Endpoint   string `mapstructure:"endpoint"`
	MaxResults int    `mapstructure:"max_results"`
}

type AppConfigType struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	AI       AIConfig       `mapstructure:"ai"`
	Agent    AgentConfig    `mapstructure:"agent"`
}

type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	Expire int    `mapstructure:"expire"`
}

var AppConfig AppConfigType

func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	viper.SetDefault("server.port", "8080")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", "3306")
	viper.SetDefault("database.username", "root")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "ai_meeting")
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("jwt.secret", "your-secret-key-here")
	viper.SetDefault("jwt.expire", 7200)
	viper.SetDefault("ai.provider", "deepseek")
	viper.SetDefault("ai.deepseek.enabled", true)
	viper.SetDefault("ai.deepseek.endpoint", "https://api.deepseek.com/chat/completions")
	viper.SetDefault("ai.deepseek.model", "deepseek-chat")
	viper.SetDefault("ai.deepseek.api_key", "")
	viper.SetDefault("ai.deepseek.api_secret", "")

	// Agent 运行时默认值
	viper.SetDefault("agent.max_steps", 8)
	viper.SetDefault("agent.tool_timeout", 15)
	viper.SetDefault("agent.total_timeout", 120)
	viper.SetDefault("agent.max_tool_result_bytes", 32*1024)
	viper.SetDefault("agent.max_parallel_tools", 4)
	viper.SetDefault("agent.sandbox_root", "./sandbox")
	viper.SetDefault("agent.context_budget", 12288) // Deprecated: 兼容旧配置
	viper.SetDefault("agent.profile_extract_every", 3)
	viper.SetDefault("agent.context.model_window_tokens", 64000)
	viper.SetDefault("agent.context.max_output_tokens", 4096)
	viper.SetDefault("agent.context.soft_limit_tokens", 16000)
	viper.SetDefault("agent.context.safety_margin_tokens", 1024)
	viper.SetDefault("agent.context.reserve_tokens", 2048)
	viper.SetDefault("agent.context.identity_max_tokens", 500)
	viper.SetDefault("agent.context.profile_max_tokens", 600)
	viper.SetDefault("agent.context.summary_max_tokens", 1000)
	viper.SetDefault("agent.context.recent_max_tokens", 6000)
	viper.SetDefault("agent.context.compress_threshold_tokens", 4800)
	viper.SetDefault("agent.search.provider", "tavily")
	viper.SetDefault("agent.search.endpoint", "https://api.tavily.com/search")
	viper.SetDefault("agent.search.api_key", "")
	viper.SetDefault("agent.search.max_results", 5)
	viper.SetDefault("agent.plan.enabled", true)
	viper.SetDefault("agent.plan.max_tasks", 6)
	viper.SetDefault("agent.plan.max_parallel", 2)
	viper.SetDefault("agent.plan.task_timeout", 60)
	viper.SetDefault("agent.plan.total_timeout", 180)
	viper.SetDefault("agent.plan.max_steps_per_task", 4)
	viper.SetDefault("agent.plan.long_query_runes", 40)
	viper.SetDefault("agent.plan.replan_enabled", true)
	viper.SetDefault("agent.plan.max_replans", 2)
	viper.SetDefault("agent.plan.retry_max_attempts", 2)
	viper.SetDefault("agent.plan.retry_backoff_ms", 500)
	viper.SetDefault("agent.plan.mode", "auto")
	viper.SetDefault("agent.plan.min_score", 2)
	viper.SetDefault("agent.plan.fast_path_runes", 24)

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		logrus.Warnf("Config file not found, using defaults: %v", err)
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		return err
	}

	logrus.Info("Config loaded successfully")
	return nil
}

func GetConfig() *AppConfigType {
	return &AppConfig
}
