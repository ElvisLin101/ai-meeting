package agentctx

// Config 控制上下文装配的 token 预算。
//
// PromptBudgetTokens 是“前缀 + 近期消息”的可用预算；工具 schema、当前 query、
// 输出预留由调用方在计算 PromptBudgetTokens 时扣除。
type Config struct {
	PromptBudgetTokens      int
	IdentityMaxTokens       int
	ProfileMaxTokens        int
	SummaryMaxTokens        int
	RecentMaxTokens         int
	CompressThresholdTokens int
}

func (c Config) withDefaults() Config {
	if c.PromptBudgetTokens <= 0 {
		c.PromptBudgetTokens = 12000
	}
	if c.IdentityMaxTokens <= 0 {
		c.IdentityMaxTokens = 500
	}
	if c.ProfileMaxTokens <= 0 {
		c.ProfileMaxTokens = 600
	}
	if c.SummaryMaxTokens <= 0 {
		c.SummaryMaxTokens = 1000
	}
	if c.RecentMaxTokens <= 0 {
		c.RecentMaxTokens = 6000
	}
	if c.CompressThresholdTokens <= 0 {
		c.CompressThresholdTokens = 4800
	}
	return c
}
