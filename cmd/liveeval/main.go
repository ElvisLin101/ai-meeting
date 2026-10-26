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
	StartedAt     string       `json:"started_at"`
	Model         string       `json:"model"`
	Total         int          `json:"total"`
	Passed        int          `json:"passed"`
	PassRate      float64      `json:"pass_rate"`
	RouteOK       int          `json:"route_ok"`
	RouteOKRate   float64      `json:"route_ok_rate"`
	PlanRuns      int          `json:"plan_runs"`
	AvgSteps      float64      `json:"avg_steps"`
	AvgTokens     float64      `json:"avg_tokens"`
	AvgDurationMs float64      `json:"avg_duration_ms"`
	Cases         []CaseResult `json:"cases"`
}

func main() {
	casesPath := flag.String("cases", "test/eval/live_cases.json", "eval cases JSON")
	outPath := flag.String("out", "tmp/live_eval_report.json", "report output")
	username := flag.String("username", "eval-bot", "eval user (isolated from real users)")
	userID := flag.String("user-id", "eval-bot", "eval user id used by interview tools")
	agentID := flag.Uint("agent-id", 1, "agent id")
	limit := flag.Int("limit", 0, "run only first N cases")
	timeout := flag.Duration("timeout", 180*time.Second, "per-case timeout")
	minPass := flag.Float64("min-pass", 0.7, "exit non-zero when pass rate is below this")
	flag.Parse()

	if err := config.LoadConfig(); err != nil {
		fatalf("load config: %v", err)
	}
	if err := mysqlrepo.InitDB(); err != nil {
		fatalf("init mysql: %v", err)
	}
	repositories.InitRedis()

	cfg := config.GetConfig()
	if strings.TrimSpace(cfg.AI.DeepSeek.APIKey) == "" {
		fmt.Println("⚠️  ai.deepseek.api_key 为空，模型可能走 mock，结果仅用于链路验证")
	}
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
		Total:     len(cases),
	}
	var totalSteps, totalTokens int
	var totalDuration int64

	for _, c := range cases {
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
		report.Cases = append(report.Cases, result)
		totalSteps += result.Steps
		totalTokens += result.Tokens
		totalDuration += result.DurationMs

		status := "FAIL"
		if result.Passed {
			status = "PASS"
		}
		fmt.Printf("[%s] %-24s plan=%-12s route_ok=%v steps=%d tokens=%d %dms\n",
			status, c.ID, result.PlanMode, result.RouteOK, result.Steps, result.Tokens, result.DurationMs)
	}

	report.PassRate = ratio(report.Passed, report.Total)
	report.RouteOKRate = ratio(report.RouteOK, report.Total)
	report.AvgSteps = ratioFloat(totalSteps, report.Total)
	report.AvgTokens = ratioFloat(totalTokens, report.Total)
	report.AvgDurationMs = ratioFloat64(totalDuration, report.Total)

	if err := writeReport(*outPath, report); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("\nsummary: pass=%d/%d (%.2f) route_ok=%d/%d (%.2f) plan=%d avg_steps=%.1f avg_tokens=%.0f avg_duration=%.0fms\n",
		report.Passed, report.Total, report.PassRate,
		report.RouteOK, report.Total, report.RouteOKRate,
		report.PlanRuns, report.AvgSteps, report.AvgTokens, report.AvgDurationMs)
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
