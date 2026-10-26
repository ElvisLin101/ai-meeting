package eval

import (
	"encoding/json"
	"os"
	"testing"

	"ai-meeting/services/agent/plan"
)

func TestBaselineCasesPass(t *testing.T) {
	report := RunEmbedded()
	if !report.OK() {
		t.Fatalf("eval baseline failed: %+v", report.Failures)
	}
	if report.RoutingTotal == 0 || report.ProfileTotal == 0 || report.ConflictTotal == 0 {
		t.Fatalf("eval cases should not be empty: %+v", report)
	}
}

func TestLiveCasesRouteExpectations(t *testing.T) {
	raw, err := os.ReadFile("../../test/eval/live_cases.json")
	if err != nil {
		t.Fatalf("read live cases: %v", err)
	}
	var file struct {
		Cases []struct {
			ID         string `json:"id"`
			Input      string `json:"input"`
			ExpectPlan string `json:"expect_plan"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse live cases: %v", err)
	}
	router := plan.NewRouter(plan.Config{Enabled: true, RouteMode: "auto"})
	for _, c := range file.Cases {
		if c.ExpectPlan == "" {
			continue
		}
		decision := router.Decide(c.Input)
		wantPlan := c.ExpectPlan == "plan"
		if decision.UsePlan != wantPlan {
			t.Errorf("live case %s: route plan=%v want=%v (%s)", c.ID, decision.UsePlan, wantPlan, decision.Reason)
		}
	}
}
