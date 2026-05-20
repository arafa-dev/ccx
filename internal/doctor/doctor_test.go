package doctor_test

import (
	"context"
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

type stubProfiles struct{}

func (stubProfiles) List(_ context.Context) ([]contracts.Profile, error) {
	return nil, nil
}
