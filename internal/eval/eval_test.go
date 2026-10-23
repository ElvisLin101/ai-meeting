package eval

import "testing"

func TestBaselineCasesPass(t *testing.T) {
	report := RunEmbedded()
	if !report.OK() {
		t.Fatalf("eval baseline failed: %+v", report.Failures)
	}
	if report.RoutingTotal == 0 || report.ProfileTotal == 0 || report.ConflictTotal == 0 {
		t.Fatalf("eval cases should not be empty: %+v", report)
	}
}
