package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/doctor"
)

func TestRunReturnsChecks(t *testing.T) {
	d := doctor.New(doctor.Deps{Profiles: stubProfiles{}})
	checks, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) == 0 {
		t.Error("expected at least one check")
	}
	for _, c := range checks {
		if c.Name == "" {
			t.Errorf("check has empty name: %+v", c)
		}
		if c.Status != "ok" && c.Status != "warn" && c.Status != "fail" {
			t.Errorf("invalid status: %q", c.Status)
		}
	}
}

func TestRunReportsInaccessibleConfigDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission checks are not portable on Windows")
	}
	parent := filepath.Join(t.TempDir(), "private")
	blocked := filepath.Join(parent, "blocked")
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:gosec // Directory execute permission is restored so t.TempDir can clean up.
		_ = os.Chmod(blocked, 0o700)
	})
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}

	inaccessibleConfig := filepath.Join(blocked, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", inaccessibleConfig)
	d := doctor.New(doctor.Deps{Profiles: stubProfiles{profiles: []contracts.Profile{{
		Name:      "work",
		ConfigDir: inaccessibleConfig,
	}}}})
	checks, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	defaultCheck := findCheck(t, checks, "default Claude Code config dir")
	if defaultCheck.Status != "fail" || !strings.Contains(defaultCheck.Detail, "cannot access") {
		t.Fatalf("default config check = %+v", defaultCheck)
	}
	profileCheck := findCheck(t, checks, "profile: work")
	if profileCheck.Status != "fail" || !strings.Contains(profileCheck.Detail, "cannot access") {
		t.Fatalf("profile config check = %+v", profileCheck)
	}
}

func findCheck(t *testing.T, checks []contracts.DoctorCheck, name string) contracts.DoctorCheck {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing check %q in %+v", name, checks)
	return contracts.DoctorCheck{}
}

type stubProfiles struct {
	profiles []contracts.Profile
}

func (s stubProfiles) List(_ context.Context) ([]contracts.Profile, error) {
	return s.profiles, nil
}
