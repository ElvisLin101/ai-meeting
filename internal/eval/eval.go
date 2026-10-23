package eval

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"ai-meeting/services/agent/context"
	"ai-meeting/services/agent/plan"
)

//go:embed cases.json
var embeddedCases []byte

// Cases 是确定性 eval 用例集。
type Cases struct {
	Routing      []RoutingCase     `json:"routing"`
	ProfileRules []ProfileRuleCase `json:"profile_rules"`
	Conflicts    []ConflictCase    `json:"conflicts"`
}

type RoutingCase struct {
	Name     string `json:"name"`
	Query    string `json:"query"`
	WantPlan bool   `json:"want_plan"`
}

type ProfileRuleCase struct {
	Name       string   `json:"name"`
	Query      string   `json:"query"`
	WantFields []string `json:"want_fields"`
}

type ConflictCase struct {
	Name            string                 `json:"name"`
	Existing        []agentctx.ProfileFact `json:"existing"`
	Add             []agentctx.ProfileFact `json:"add"`
	Force           bool                   `json:"force"`
	WantActiveValue string                 `json:"want_active_value"`
	WantQuestions   int                    `json:"want_questions"`
	WantActiveCount int                    `json:"want_active_count"`
}

// Report 是一次 eval 的结果。
type Report struct {
	RoutingTotal   int      `json:"routing_total"`
	RoutingPassed  int      `json:"routing_passed"`
	ProfileTotal   int      `json:"profile_total"`
	ProfilePassed  int      `json:"profile_passed"`
	ConflictTotal  int      `json:"conflict_total"`
	ConflictPassed int      `json:"conflict_passed"`
	Failures       []string `json:"failures,omitempty"`
}

// OK 表示所有用例通过。
func (r Report) OK() bool { return len(r.Failures) == 0 }

// LoadCases 读取内置用例。
func LoadCases() (Cases, error) {
	var cases Cases
	if err := json.Unmarshal(embeddedCases, &cases); err != nil {
		return cases, err
	}
	return cases, nil
}

// RunEmbedded 运行内置用例。
func RunEmbedded() Report {
	cases, err := LoadCases()
	if err != nil {
		return Report{Failures: []string{"load cases: " + err.Error()}}
	}
	return Run(cases)
}

// Run 执行路由、画像规则和画像冲突三组确定性用例。
func Run(cases Cases) Report {
	var report Report

	router := plan.NewRouter(plan.Config{Enabled: true, RouteMode: "auto"})
	for _, c := range cases.Routing {
		report.RoutingTotal++
		decision := router.Decide(c.Query)
		if decision.UsePlan == c.WantPlan {
			report.RoutingPassed++
			continue
		}
		report.Failures = append(report.Failures, fmt.Sprintf(
			"routing/%s: query=%q got plan=%v want=%v (%s)",
			c.Name, c.Query, decision.UsePlan, c.WantPlan, decision.Reason))
	}

	for _, c := range cases.ProfileRules {
		report.ProfileTotal++
		candidates := agentctx.ExtractRuleCandidates(c.Query)
		got := make(map[string]bool, len(candidates))
		for _, item := range candidates {
			got[item.Field] = true
		}
		if sameStringSet(got, c.WantFields) {
			report.ProfilePassed++
			continue
		}
		report.Failures = append(report.Failures, fmt.Sprintf(
			"profile_rules/%s: query=%q got=%v want=%v", c.Name, c.Query, keys(got), c.WantFields))
	}

	for _, c := range cases.Conflicts {
		report.ConflictTotal++
		profile := &agentctx.UserProfileData{Version: 1, Facts: c.Existing}
		agentctx.NormalizeProfile(profile)
		next, questions, _, err := agentctx.ApplyProfileDiff(profile, agentctx.ProfileDiff{
			Force: c.Force,
			Add:   c.Add,
		})
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("conflict/%s: apply error: %v", c.Name, err))
			continue
		}
		active := next.ActiveFacts()
		if len(questions) != c.WantQuestions {
			report.Failures = append(report.Failures, fmt.Sprintf(
				"conflict/%s: questions=%d want=%d", c.Name, len(questions), c.WantQuestions))
			continue
		}
		if c.WantActiveCount > 0 && len(active) != c.WantActiveCount {
			report.Failures = append(report.Failures, fmt.Sprintf(
				"conflict/%s: active_count=%d want=%d", c.Name, len(active), c.WantActiveCount))
			continue
		}
		if !hasActiveValue(active, c.Add, c.WantActiveValue) {
			report.Failures = append(report.Failures, fmt.Sprintf(
				"conflict/%s: active value not %q: %+v", c.Name, c.WantActiveValue, active))
			continue
		}
		report.ConflictPassed++
	}

	return report
}

func hasActiveValue(active []agentctx.ProfileFact, add []agentctx.ProfileFact, want string) bool {
	if len(add) == 0 {
		return true
	}
	field, key := add[0].Field, add[0].Key
	for _, f := range active {
		if f.Field == field && f.Key == key {
			return f.Value == want
		}
	}
	return false
}

func sameStringSet(got map[string]bool, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, v := range want {
		if !got[v] {
			return false
		}
	}
	return true
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
