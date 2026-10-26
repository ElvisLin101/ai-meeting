package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-meeting/config"
	"ai-meeting/repositories"
	mysqlrepo "ai-meeting/repositories/mysql"
	agentrun "ai-meeting/services/agent/run"
)

// live eval 用真实模型和真实 MySQL/Redis 跑一组任务，输出 direct/plan 的
// 成功率、步数、token、延迟。它不在 CI 跑，需要 config/config.yaml、
// MySQL、Redis 和可用模型 key。

type Case struct {
	ID             string   `json:"id"`
	Input          string   `json:"input"`
	ExpectPlan     string   `json:"expect_plan"` // plan / direct / ""
	MustContain    []string `json:"must_contain"`
	MustNotContain []string `json:"must_not_contain"`
	Rubric         []string `json:"rubric"`
	MaxSteps       int      `json:"max_steps"`
}

type CasesFile struct {
	Cases []Case `json:"cases"`
}

type CaseResult struct {
	ID            string   `json:"id"`
	Input         string   `json:"input"`
	PlanMode      string   `json:"plan_mode"`
	ExpectPlan    string   `json:"expect_plan,omitempty"`
	RouteOK       bool     `json:"route_ok"`
	Passed        bool     `json:"passed"`
	Status        string   `json:"status"`
	StopReason    string   `json:"stop_reason"`
	Steps         int      `json:"steps"`
	Tokens        int      `json:"tokens"`
	DurationMs    int64    `json:"duration_ms"`
	Quality       float64  `json:"quality"`
	Missing       []string `json:"missing,omitempty"`
	Forbidden     []string `json:"forbidden,omitempty"`
	OutputPreview string   `json:"output_preview,omitempty"`
	RunID         string   `json:"run_id,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type Report struct {
	StartedAt       string       `json:"started_at"`
	Model           string       `json:"model"`
	RunLabel        string       `json:"run_label,omitempty"`
	User            string       `json:"user,omitempty"`
	Sandbox         string       `json:"sandbox,omitempty"`
	Total           int          `json:"total"`
	Passed          int          `json:"passed"`
	PassRate        float64      `json:"pass_rate"`
	RouteOK         int          `json:"route_ok"`
	RouteOKRate     float64      `json:"route_ok_rate"`
	PlanRuns        int          `json:"plan_runs"`
	Completed       int          `json:"completed"`
	CompletedRate   float64      `json:"completed_rate"`
	QualityPass     int          `json:"quality_pass"`
	QualityPassRate float64      `json:"quality_pass_rate"`
	AvgQuality      float64      `json:"avg_quality"`
	AvgSteps        float64      `json:"avg_steps"`
	AvgTokens       float64      `json:"avg_tokens"`
	AvgDurationMs   float64      `json:"avg_duration_ms"`
	Cases           []CaseResult `json:"cases"`
}

func main() {
	casesPath := flag.String("cases", "test/eval/live_cases.json", "eval cases JSON")
	outPath := flag.String("out", "tmp/live_eval_report.json", "report output")
	username := flag.String("username", "eval-bot", "eval user (isolated from real users)")
	userID := flag.String("user-id", "eval-bot", "eval user id used by interview tools")
	isolate := flag.Bool("isolate", true, "use a fresh eval user and sandbox for this run")
	label := flag.String("label", "", "label for isolated run (default timestamp-pid)")
	agentID := flag.Uint("agent-id", 0, "agent id (0 uses config/config.yaml, >0 uses ai_properties)")
	limit := flag.Int("limit", 0, "run only first N cases")
	timeout := flag.Duration("timeout", 180*time.Second, "per-case timeout")
	minPass := flag.Float64("min-pass", 0.7, "exit non-zero when pass rate is below this")
	qualityThreshold := flag.Float64("quality-threshold", 0.7, "rubric coverage threshold for quality_pass")
	flag.Parse()

	if err := config.LoadConfig(); err != nil {
		fatalf("load config: %v", err)
	}
	cfg := config.GetConfig()
	if strings.TrimSpace(cfg.AI.DeepSeek.APIKey) == "" && *agentID == 0 {
		fatalf(`ai.deepseek.api_key 未配置，Live Eval 需要真实模型。

任选一种方式配置后重跑：
  1. 编辑 config/config.yaml，填写 ai.deepseek.api_key
  2. 使用环境变量：AI_DEEPSEEK_API_KEY=sk-xxx go run ./cmd/liveeval -limit 1

注意：config/config.yaml 已被 .gitignore 忽略，不要提交或粘贴到对话里。
如果模型配置在 MySQL ai_properties 表里，请改用 -agent-id <id>。`)
	}

	runLabel := strings.TrimSpace(*label)
	if *isolate {
		if runLabel == "" {
			runLabel = fmt.Sprintf("eval-%s-%d", time.Now().Format("20060102-150405"), os.Getpid())
		}
		runLabel = sanitizeLabel(runLabel)
		*username = runLabel
		*userID = runLabel
		sandboxRoot := filepath.Join("tmp", "eval-runs", runLabel, "sandbox")
		abs, err := filepath.Abs(sandboxRoot)
		if err != nil {
			fatalf("resolve eval sandbox: %v", err)
		}
		if err := resetSandbox(abs); err != nil {
			fatalf("reset eval sandbox: %v", err)
		}
		cfg.Agent.SandboxRoot = abs
		fmt.Printf("isolated eval: user=%s sandbox=%s\n", runLabel, abs)
	} else if runLabel == "" {
		runLabel = "shared"
	}

	if err := mysqlrepo.InitDB(); err != nil {
		fatalf("init mysql: %v", err)
	}
	repositories.InitRedis()

	fmt.Printf("model: %s\n", cfg.AI.DeepSeek.Model)

	cases, err := loadCases(*casesPath)
	if err != nil {
		fatalf("load cases: %v", err)
	}
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}
	if len(cases) == 0 {
		fatalf("no cases")
	}

	svc := agentrun.GetService()
	report := Report{
		StartedAt: time.Now().Format("2006-01-02 15:04:05"),
		Model:     cfg.AI.DeepSeek.Model,
		RunLabel:  runLabel,
		User:      *username,
		Sandbox:   cfg.Agent.SandboxRoot,
		Total:     len(cases),
	}
	var totalSteps, totalTokens int
	var totalDuration int64
	var totalQuality float64

	for _, c := range cases {
		if *isolate {
			if err := resetSandbox(cfg.Agent.SandboxRoot); err != nil {
				fatalf("reset sandbox before %s: %v", c.ID, err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		start := time.Now()
		res, runErr := svc.Start(ctx, agentrun.StartRequest{
			UserID: *userID, Username: *username, AgentID: uint(*agentID), Input: c.Input,
		})
		elapsed := time.Since(start).Milliseconds()
		cancel()

		result := scoreCase(c, res, runErr)
		result.DurationMs = elapsed
		if res != nil {
			result.RunID = res.RunID
			result.PlanMode = res.PlanMode
			result.Status = res.Status
			result.StopReason = res.StopReason
			result.Steps = res.Steps
			result.Tokens = res.Tokens
			result.OutputPreview = preview(res.Output, 500)
			if strings.HasPrefix(res.PlanMode, "plan") {
				report.PlanRuns++
			}
		}
		result.RouteOK = routeOK(c.ExpectPlan, result.PlanMode)
		if result.RouteOK {
			report.RouteOK++
		}
		if result.Passed {
			report.Passed++
		}
		if result.Status == "completed" {
			report.Completed++
		}
		if result.Quality >= *qualityThreshold {
			report.QualityPass++
		}
		report.Cases = append(report.Cases, result)
		totalSteps += result.Steps
		totalTokens += result.Tokens
		totalDuration += result.DurationMs
		totalQuality += result.Quality

		status := "FAIL"
		if result.Passed {
			status = "PASS"
		}
		fmt.Printf("[%s] %-24s plan=%-12s status=%-9s route_ok=%v steps=%d tokens=%d %dms quality=%.2f\n",
			status, c.ID, result.PlanMode, result.Status, result.RouteOK,
			result.Steps, result.Tokens, result.DurationMs, result.Quality)
	}

	report.PassRate = ratio(report.Passed, report.Total)
	report.RouteOKRate = ratio(report.RouteOK, report.Total)
	report.CompletedRate = ratio(report.Completed, report.Total)
	report.QualityPassRate = ratio(report.QualityPass, report.Total)
	report.AvgQuality = totalQuality / float64(report.Total)
	report.AvgSteps = ratioFloat(totalSteps, report.Total)
	report.AvgTokens = ratioFloat(totalTokens, report.Total)
	report.AvgDurationMs = ratioFloat64(totalDuration, report.Total)

	if err := writeReport(*outPath, report); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("\nsummary: pass=%d/%d (%.2f) route_ok=%d/%d (%.2f) plan=%d completed=%d/%d quality_pass=%d/%d avg_quality=%.2f avg_steps=%.1f avg_tokens=%.0f avg_duration=%.0fms\n",
		report.Passed, report.Total, report.PassRate,
		report.RouteOK, report.Total, report.RouteOKRate,
		report.PlanRuns, report.Completed, report.Total,
		report.QualityPass, report.Total, report.AvgQuality,
		report.AvgSteps, report.AvgTokens, report.AvgDurationMs)
	fmt.Printf("report: %s\n", *outPath)

	if report.PassRate < *minPass {
		os.Exit(1)
	}
}

func loadCases(path string) ([]Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file CasesFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	return file.Cases, nil
}

func scoreCase(c Case, res *agentrun.StartResult, runErr error) CaseResult {
	result := CaseResult{ID: c.ID, Input: c.Input, ExpectPlan: c.ExpectPlan}
	if runErr != nil {
		result.Error = runErr.Error()
		return result
	}
	if res == nil {
		result.Error = "empty result"
		return result
	}
	for _, want := range c.MustContain {
		if !strings.Contains(res.Output, want) {
			result.Missing = append(result.Missing, want)
		}
	}
	for _, bad := range c.MustNotContain {
		if strings.Contains(res.Output, bad) {
			result.Forbidden = append(result.Forbidden, bad)
		}
	}
	if c.MaxSteps > 0 && res.Steps > c.MaxSteps {
		result.Forbidden = append(result.Forbidden, fmt.Sprintf("steps>%d", c.MaxSteps))
	}
	if len(c.Rubric) > 0 {
		matched := 0
		for _, item := range c.Rubric {
			if strings.Contains(res.Output, item) {
				matched++
			}
		}
		result.Quality = ratio(matched, len(c.Rubric))
	} else {
		result.Quality = 1
	}
	result.Passed = len(result.Missing) == 0 && len(result.Forbidden) == 0
	return result
}

func routeOK(expect, actual string) bool {
	if expect == "" {
		return true
	}
	if expect == "plan" {
		return strings.HasPrefix(actual, "plan")
	}
	return actual == expect
}

func preview(text string, maxRunes int) string {
	r := []rune(text)
	if len(r) <= maxRunes {
		return text
	}
	return string(r[:maxRunes]) + "…"
}

func writeReport(path string, report Report) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// sanitizeLabel 把用户传入的 label 限制为文件系统安全的字符。
func sanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "eval"
	}
	return out
}

// resetSandbox 只允许清空 tmp/eval-runs 下的隔离目录，避免误删真实 sandbox。
func resetSandbox(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty sandbox path")
	}
	if !strings.Contains(filepath.ToSlash(path), "/eval-runs/") {
		return fmt.Errorf("refusing to reset non-eval sandbox: %s", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func ratioFloat(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func ratioFloat64(a int64, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
