package tui_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/tui"
)

func TestPickProfileNoTTYReturnsFirst(t *testing.T) {
	got, err := tui.PickProfile([]contracts.Profile{
		{Name: "work", ConfigDir: "/tmp/w"},
		{Name: "personal", ConfigDir: "/tmp/p"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "work" {
		t.Errorf("want first profile, got %q", got.Name)
	}
}

func TestPickProfileEmpty(t *testing.T) {
	_, err := tui.PickProfile(nil)
	if err == nil {
		t.Errorf("want error for empty profile list")
	}
}
