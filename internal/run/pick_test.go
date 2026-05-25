package run_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/run"
)

type stubEvaluator struct {
	result headroom.Result
	err    error
}

func (s stubEvaluator) Evaluate(_ context.Context, _ []contracts.Profile, _ headroom.Options) (headroom.Result, error) {
	return s.result, s.err
}

func TestPickReturnsExplicitProfileWhenProvided(t *testing.T) {
	profiles := []contracts.Profile{{Name: "work"}, {Name: "personal"}}
	got, why, err := run.Pick(context.Background(), run.PickOptions{
		Profiles: profiles,
		Override: "personal",
	})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "personal" {
		t.Errorf("Override ignored: got %q, want personal", got.Name)
	}
	if why != "explicit --profile personal" {
		t.Errorf("Why: got %q", why)
	}
}

func TestPickOverrideUnknownProfileErrors(t *testing.T) {
	_, _, err := run.Pick(context.Background(), run.PickOptions{
		Profiles: []contracts.Profile{{Name: "work"}},
		Override: "ghost",
	})
	if err == nil {
		t.Fatal("expected error for unknown override")
	}
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("error: got %v, want ErrProfileNotFound wrap", err)
	}
}

func TestPickFallsBackToEvaluatorRecommendation(t *testing.T) {
	profiles := []contracts.Profile{{Name: "work"}, {Name: "personal"}}
	ev := stubEvaluator{result: headroom.Result{
		Recommendation: &headroom.Candidate{Profile: "work", Available: true, Score: 50},
		Candidates: []headroom.Candidate{
			{Profile: "work", Available: true, Score: 50},
			{Profile: "personal", Available: true, Score: 30},
		},
	}}
	got, why, err := run.Pick(context.Background(), run.PickOptions{Profiles: profiles, Evaluator: ev})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "work" {
		t.Errorf("expected recommendation: got %q", got.Name)
	}
	if why == "" {
		t.Errorf("expected non-empty why")
	}
}

func TestPickNoAvailableProfilesReturnsError(t *testing.T) {
	profiles := []contracts.Profile{{Name: "work"}}
	ev := stubEvaluator{result: headroom.Result{
		Candidates: []headroom.Candidate{{Profile: "work", Available: false, Reasons: []string{"hard cap"}}},
	}}
	_, _, err := run.Pick(context.Background(), run.PickOptions{Profiles: profiles, Evaluator: ev})
	if err == nil {
		t.Fatal("expected error when nothing recommendable")
	}
	if !errors.Is(err, run.ErrNoRecommendation) {
		t.Errorf("error: got %v, want ErrNoRecommendation wrap", err)
	}
}

func TestPickNilEvaluatorReturnsError(t *testing.T) {
	_, _, err := run.Pick(context.Background(), run.PickOptions{
		Profiles: []contracts.Profile{{Name: "work"}},
	})
	if err == nil {
		t.Fatal("expected error for nil evaluator")
	}
	if !errors.Is(err, run.ErrNoRecommendation) {
		t.Errorf("error: got %v, want ErrNoRecommendation wrap", err)
	}
}

func TestPickRecommendationMissingFromRegistryErrors(t *testing.T) {
	profiles := []contracts.Profile{{Name: "work"}}
	ev := stubEvaluator{result: headroom.Result{
		Recommendation: &headroom.Candidate{Profile: "ghost", Available: true},
	}}
	_, _, err := run.Pick(context.Background(), run.PickOptions{Profiles: profiles, Evaluator: ev})
	if err == nil {
		t.Fatal("expected error for recommendation outside registry")
	}
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("error: got %v, want ErrProfileNotFound wrap", err)
	}
}

func TestPickEmptyProfileListReturnsError(t *testing.T) {
	_, _, err := run.Pick(context.Background(), run.PickOptions{})
	if err == nil {
		t.Fatal("expected error for empty profile list")
	}
}
