package contracts_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestProfileJSONRoundtrip(t *testing.T) {
	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/Users/arafa/.claude-profiles/work",
		Label:      "Work account",
		Color:      "#3B82F6",
		CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out contracts.Profile
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out != in {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", out, in)
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
