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
