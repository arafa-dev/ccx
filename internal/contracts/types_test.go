package contracts_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestProfileJSONRoundtrip(t *testing.T) {
	suggest := false
	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/Users/arafa/.claude-profiles/work",
		Label:      "Work account",
		Color:      "#3B82F6",
		CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
		Limits: contracts.ProfileLimits{
			DailyTokenBudget:  120000,
			WeeklyTokenBudget: 500000,
			MonthlyUSDBudget:  150.50,
			Priority:          10,
			SuggestEnabled:    &suggest,
			RateLimitCooldown: "2h30m",
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out contracts.Profile
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(out, in) {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", out, in)
	}
}

func TestProfileLimitsJSONUsesContractFieldNames(t *testing.T) {
	suggest := true
	p := contracts.Profile{
		Name:      "work",
		ConfigDir: "/Users/arafa/.claude-profiles/work",
		Limits: contracts.ProfileLimits{
			DailyTokenBudget:  1000,
			WeeklyTokenBudget: 7000,
			MonthlyUSDBudget:  99.95,
			Priority:          -2,
			SuggestEnabled:    &suggest,
			RateLimitCooldown: "45m",
		},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal fields: %v", err)
	}
	if _, ok := fields["limits"]; !ok {
		t.Fatalf("marshaled Profile missing limits field in %s", data)
	}
	var limits map[string]json.RawMessage
	if err := json.Unmarshal(fields["limits"], &limits); err != nil {
		t.Fatalf("unmarshal limits: %v", err)
	}
	for _, name := range []string{"daily_token_budget", "weekly_token_budget", "monthly_usd_budget", "priority", "suggest_enabled", "rate_limit_cooldown"} {
		if _, ok := limits[name]; !ok {
			t.Errorf("marshaled ProfileLimits missing field %q in %s", name, fields["limits"])
		}
	}
	for _, name := range []string{"DailyTokenBudget", "WeeklyTokenBudget", "MonthlyUSDBudget", "SuggestEnabled", "RateLimitCooldown"} {
		if _, ok := limits[name]; ok {
			t.Errorf("marshaled ProfileLimits included Go field name %q in %s", name, fields["limits"])
		}
	}
}

func TestProfileLimitsPlanFieldsRoundtrip(t *testing.T) {
	in := contracts.ProfileLimits{
		DailyTokenBudget: 1_000_000,
		PlanTier:         "max20",
		WeeklyAnchor:     "monday",
		Caps5hTurns:      900,
		CapsWeeklyTurns:  4500,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal fields: %v", err)
	}
	for _, name := range []string{"plan_tier", "weekly_anchor", "caps_5h_turns", "caps_weekly_turns"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled ProfileLimits missing field %q in %s", name, data)
		}
	}
	for _, name := range []string{"PlanTier", "WeeklyAnchor", "Caps5hTurns", "CapsWeeklyTurns"} {
		if _, ok := fields[name]; ok {
			t.Errorf("marshaled ProfileLimits included Go field name %q in %s", name, data)
		}
	}
	var out contracts.ProfileLimits
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.PlanTier != "max20" {
		t.Errorf("PlanTier = %q, want %q", out.PlanTier, "max20")
	}
	if out.WeeklyAnchor != "monday" {
		t.Errorf("WeeklyAnchor = %q, want %q", out.WeeklyAnchor, "monday")
	}
	if out.Caps5hTurns != 900 {
		t.Errorf("Caps5hTurns = %d, want 900", out.Caps5hTurns)
	}
	if out.CapsWeeklyTurns != 4500 {
		t.Errorf("CapsWeeklyTurns = %d, want 4500", out.CapsWeeklyTurns)
	}
}

func TestProfileLimitsPlanFieldsOmitEmpty(t *testing.T) {
	in := contracts.ProfileLimits{DailyTokenBudget: 100}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{"plan_tier", "weekly_anchor", "caps_5h_turns", "caps_weekly_turns", "PlanTier", "WeeklyAnchor", "Caps5hTurns", "CapsWeeklyTurns"} {
		if strings.Contains(s, key) {
			t.Errorf("expected %q to be omitted from %q", key, s)
		}
	}
}

func TestTelemetryJSONUsesContractFieldNames(t *testing.T) {
	ts := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	status := contracts.DaemonStatus{
		PID:             1234,
		Version:         "v0.1.0",
		StartedAt:       ts,
		Port:            17333,
		URL:             "http://127.0.0.1:17333",
		DBPath:          "/tmp/ccx.db",
		LogPath:         "/tmp/ccx.log",
		ProcessIdentity: "test-process-identity",
		ProfilesWatched: 3,
		Running:         true,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal DaemonStatus: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal DaemonStatus fields: %v", err)
	}
	for _, name := range []string{"pid", "version", "started_at", "port", "url", "db_path", "log_path", "process_identity", "profiles_watched", "running"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled DaemonStatus missing field %q in %s", name, data)
		}
	}

	hook := contracts.HookEvent{
		Profile:      "work",
		Session:      "session-1",
		Event:        "StopFailure",
		Timestamp:    ts,
		Transcript:   "/tmp/transcript.jsonl",
		CWD:          "/repo",
		Model:        "claude-opus-4-7",
		Source:       "hook",
		Permission:   "acceptEdits",
		Reason:       "rate-limit",
		Error:        "429",
		ErrorDetails: "too many requests",
		Trigger:      "stop",
	}

	data, err = json.Marshal(hook)
	if err != nil {
		t.Fatalf("marshal HookEvent: %v", err)
	}
	fields = map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal HookEvent fields: %v", err)
	}
	for _, name := range []string{"profile", "session", "event", "timestamp", "transcript", "cwd", "model", "source", "permission", "reason", "error", "error_details", "trigger"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled HookEvent missing field %q in %s", name, data)
		}
	}

	session := contracts.SessionTelemetry{
		Profile:        "work",
		Session:        "session-1",
		LastSeenAt:     ts,
		Status:         "failed",
		FailureError:   "429",
		FailureDetails: "too many requests",
		CompactCount:   1,
	}
	data, err = json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal SessionTelemetry: %v", err)
	}
	fields = map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal SessionTelemetry fields: %v", err)
	}
	for _, name := range []string{"profile", "session", "last_seen_at", "status", "failure_error", "failure_details", "compact_count"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled SessionTelemetry missing field %q in %s", name, data)
		}
	}

	health := contracts.ProfileHealth{Profile: "work", CheckedAt: ts, AuthStatus: "ok", AuthDetail: "valid"}
	data, err = json.Marshal(health)
	if err != nil {
		t.Fatalf("marshal ProfileHealth: %v", err)
	}
	fields = map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal ProfileHealth fields: %v", err)
	}
	for _, name := range []string{"profile", "checked_at", "auth_status", "auth_detail"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled ProfileHealth missing field %q in %s", name, data)
		}
	}

	rec := contracts.HeadroomRecommendation{
		Profile:         "work",
		Score:           0.92,
		HeadroomPercent: 77.5,
		Available:       true,
		Reason:          "healthy",
		CooldownUntil:   ts,
		AuthStatus:      "ok",
	}
	data, err = json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal HeadroomRecommendation: %v", err)
	}
	fields = map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal HeadroomRecommendation fields: %v", err)
	}
	for _, name := range []string{"profile", "score", "headroom_percent", "available", "reason", "cooldown_until", "auth_status"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled HeadroomRecommendation missing field %q in %s", name, data)
		}
	}

	q := contracts.SessionQuery{Profile: "work", Status: "failed", Since: ts, Limit: 5}
	data, err = json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal SessionQuery: %v", err)
	}
	fields = map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal SessionQuery fields: %v", err)
	}
	for _, name := range []string{"profile", "status", "since", "limit"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled SessionQuery missing field %q in %s", name, data)
		}
	}
}

func TestProfileZeroValueIsUsable(t *testing.T) {
	var p contracts.Profile
	if p.Name != "" {
		t.Errorf("zero Profile.Name should be empty, got %q", p.Name)
	}
}

func TestUsageAdd(t *testing.T) {
	a := contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 25}
	b := contracts.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 20, CacheCreateTokens: 1}

	got := a.Add(b)
	want := contracts.Usage{InputTokens: 110, OutputTokens: 55, CacheReadTokens: 220, CacheCreateTokens: 26}

	if got != want {
		t.Errorf("Add mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestUsageTotalTokens(t *testing.T) {
	u := contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 25}
	if got, want := u.TotalTokens(), 375; got != want {
		t.Errorf("TotalTokens: got %d want %d", got, want)
	}
}

func TestUsageRowJSONUsesContractFieldNames(t *testing.T) {
	row := contracts.UsageRow{
		Profile:      "work",
		Project:      "ccx",
		Model:        "claude-opus-4-7",
		Day:          time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
		Usage:        contracts.Usage{InputTokens: 100, OutputTokens: 50},
		SessionCount: 2,
		EstimatedUSD: 0.42,
	}

	data, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal fields: %v", err)
	}

	for _, name := range []string{"profile", "project", "model", "day", "usage", "session_count", "estimated_usd"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("marshaled UsageRow missing field %q in %s", name, data)
		}
	}
	for _, name := range []string{"Profile", "Project", "Model", "Day", "Usage", "SessionCount", "EstimatedUSD"} {
		if _, ok := fields[name]; ok {
			t.Errorf("marshaled UsageRow included Go field name %q in %s", name, data)
		}
	}
}

func TestEventJSONRoundtrip(t *testing.T) {
	in := contracts.Event{
		UUID:      "01H7Z8...",
		SessionID: "sess-abc",
		Timestamp: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Type:      "assistant",
		Project:   "ccx",
		Model:     "claude-opus-4-7",
		Usage: &contracts.Usage{
			InputTokens:       1000,
			OutputTokens:      200,
			CacheReadTokens:   5000,
			CacheCreateTokens: 100,
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out contracts.Event
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.UUID != in.UUID || out.Type != in.Type || out.Usage == nil || *out.Usage != *in.Usage {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", out, in)
	}
}

func TestTimeRangeContains(t *testing.T) {
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)
	tr := contracts.TimeRange{Start: start, End: end}

	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"before", time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), false},
		{"at start", start, true},
		{"middle", time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC), true},
		{"at end", end, true},
		{"after", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tr.Contains(tc.t); got != tc.want {
				t.Errorf("Contains(%v) = %v want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestUsageQueryDefaults(t *testing.T) {
	q := contracts.UsageQuery{}
	if q.Profile != "" {
		t.Errorf("default Profile should be empty (means all), got %q", q.Profile)
	}
}

func TestParseShell(t *testing.T) {
	tests := []struct {
		in   string
		want contracts.Shell
		ok   bool
	}{
		{"zsh", contracts.ShellZsh, true},
		{"bash", contracts.ShellBash, true},
		{"fish", contracts.ShellFish, true},
		{"pwsh", contracts.ShellPowerShell, true},
		{"powershell", contracts.ShellPowerShell, true},
		{"unknown", contracts.ShellUnknown, false},
		{"", contracts.ShellUnknown, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := contracts.ParseShell(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("ParseShell(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestShellString(t *testing.T) {
	if got, want := contracts.ShellZsh.String(), "zsh"; got != want {
		t.Errorf("ShellZsh.String() = %q want %q", got, want)
	}
}
