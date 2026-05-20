package storage

import (
	"strings"
	"testing"
)

func TestBuildDSNLeavesPragmaArgumentsLiteral(t *testing.T) {
	got := buildDSN("/tmp/ccx.db")
	for _, want := range []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=foreign_keys(on)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=busy_timeout(5000)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildDSN() = %q, missing literal %q", got, want)
		}
	}
	if strings.Contains(got, "%28") || strings.Contains(got, "%29") {
		t.Errorf("buildDSN() percent-encoded pragma parentheses: %q", got)
	}
}
