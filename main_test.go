package main

import "testing"

func TestValidateJWTSecret(t *testing.T) {
	bad := []string{"", "your-secret-key-here", "short"}
	for _, s := range bad {
		if err := validateJWTSecret(s); err == nil {
			t.Fatalf("secret %q should be rejected", s)
		}
	}
	if err := validateJWTSecret("a-sufficiently-long-secret"); err != nil {
		t.Fatalf("valid secret rejected: %v", err)
	}
}
