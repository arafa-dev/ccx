package profile_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
)

func TestValidateProfileRejectsEmptyName(t *testing.T) {
	p := contracts.Profile{
		Name:      "",
		ConfigDir: "/abs/path",
	}
	err := profile.ValidateProfile(p)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateProfileRejectsRelativeConfigDir(t *testing.T) {
	p := contracts.Profile{Name: "work", ConfigDir: "relative/path"}
	err := profile.ValidateProfile(p)
	if !errors.Is(err, contracts.ErrInvalidConfigDir) {
		t.Fatalf("expected ErrInvalidConfigDir, got %v", err)
	}
}

func TestValidateProfileRejectsEmptyConfigDir(t *testing.T) {
	p := contracts.Profile{Name: "work", ConfigDir: ""}
	err := profile.ValidateProfile(p)
	if !errors.Is(err, contracts.ErrInvalidConfigDir) {
		t.Fatalf("expected ErrInvalidConfigDir, got %v", err)
	}
}

func TestValidateProfileAcceptsAbsolutePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "claude")
	p := contracts.Profile{
		Name:       "work",
		ConfigDir:  abs,
		CreatedAt:  time.Now().UTC(),
		LastUsedAt: time.Now().UTC(),
	}
	if err := profile.ValidateProfile(p); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateProfileRejectsNamesWithSlashOrSpace(t *testing.T) {
	for _, name := range []string{"foo/bar", "foo bar", "foo\tbar", "."} {
		p := contracts.Profile{Name: name, ConfigDir: "/abs/x"}
		if err := profile.ValidateProfile(p); err == nil {
			t.Errorf("expected error for name %q, got nil", name)
		}
	}
}

func TestValidateProfileNameRegex(t *testing.T) {
	for _, name := range []string{"Work", "my_profile", "abc!"} {
		p := contracts.Profile{Name: name, ConfigDir: "/abs/x"}
		if err := profile.ValidateProfile(p); err == nil {
			t.Errorf("expected error for name %q, got nil", name)
		}
	}

	p := contracts.Profile{Name: "123-ok", ConfigDir: filepath.Join(t.TempDir(), "x")}
	if err := profile.ValidateProfile(p); err != nil {
		t.Fatalf("expected valid name, got %v", err)
	}
}

func TestValidateProfileRejectsNegativeLimits(t *testing.T) {
	tests := []struct {
		name   string
		limits contracts.ProfileLimits
	}{
		{"daily token budget", contracts.ProfileLimits{DailyTokenBudget: -1}},
		{"weekly token budget", contracts.ProfileLimits{WeeklyTokenBudget: -1}},
		{"monthly usd budget", contracts.ProfileLimits{MonthlyUSDBudget: -0.01}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := contracts.Profile{
				Name:      "work",
				ConfigDir: filepath.Join(t.TempDir(), "x"),
				Limits:    tc.limits,
			}
			if err := profile.ValidateProfile(p); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestValidateProfileRejectsInvalidRateLimitCooldown(t *testing.T) {
	p := contracts.Profile{
		Name:      "work",
		ConfigDir: filepath.Join(t.TempDir(), "x"),
		Limits: contracts.ProfileLimits{
			RateLimitCooldown: "not-a-duration",
		},
	}
	if err := profile.ValidateProfile(p); err == nil {
		t.Fatal("expected validation error for invalid cooldown")
	}
}

func TestValidateProfileAcceptsProfileLimits(t *testing.T) {
	suggest := true
	p := contracts.Profile{
		Name:      "work",
		ConfigDir: filepath.Join(t.TempDir(), "x"),
		Limits: contracts.ProfileLimits{
			DailyTokenBudget:  1000,
			WeeklyTokenBudget: 7000,
			MonthlyUSDBudget:  12.50,
			Priority:          -10,
			SuggestEnabled:    &suggest,
			RateLimitCooldown: "15m",
		},
	}
	if err := profile.ValidateProfile(p); err != nil {
		t.Fatalf("expected valid limits, got %v", err)
	}
}
