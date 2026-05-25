package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
)

// ErrNoProfiles reports that no profiles are available to pick from.
var ErrNoProfiles = errors.New("run: no profiles registered")

// ErrNoRecommendation reports that headroom evaluation found no usable profile.
var ErrNoRecommendation = errors.New("run: no recommendable profile")

// EvaluatorFunc evaluates profile headroom for automatic profile selection.
type EvaluatorFunc interface {
	Evaluate(ctx context.Context, profiles []contracts.Profile, opts headroom.Options) (headroom.Result, error)
}

// PickOptions controls profile selection for a run.
type PickOptions struct {
	Profiles  []contracts.Profile
	Override  string
	Evaluator EvaluatorFunc
}

// Pick selects the profile to use for a run and returns a human-readable reason.
func Pick(ctx context.Context, opts PickOptions) (contracts.Profile, string, error) {
	if len(opts.Profiles) == 0 {
		return contracts.Profile{}, "", ErrNoProfiles
	}

	if opts.Override != "" {
		profile, ok := findProfile(opts.Profiles, opts.Override)
		if !ok {
			return contracts.Profile{}, "", fmt.Errorf("profile %q: %w", opts.Override, contracts.ErrProfileNotFound)
		}
		return profile, fmt.Sprintf("explicit --profile %s", opts.Override), nil
	}

	if opts.Evaluator == nil {
		return contracts.Profile{}, "", fmt.Errorf("%w: evaluator is nil", ErrNoRecommendation)
	}

	result, err := opts.Evaluator.Evaluate(ctx, opts.Profiles, headroom.Options{})
	if err != nil {
		return contracts.Profile{}, "", fmt.Errorf("evaluating headroom: %w", err)
	}
	if result.Recommendation == nil {
		return contracts.Profile{}, "", ErrNoRecommendation
	}

	profile, ok := findProfile(opts.Profiles, result.Recommendation.Profile)
	if !ok {
		return contracts.Profile{}, "", fmt.Errorf("recommended profile %q: %w", result.Recommendation.Profile, contracts.ErrProfileNotFound)
	}

	why := fmt.Sprintf(
		"headroom recommendation: score=%.1f headroom=%.1f%%",
		result.Recommendation.Score,
		result.Recommendation.HeadroomPercent,
	)
	return profile, why, nil
}

func findProfile(profiles []contracts.Profile, name string) (contracts.Profile, bool) {
	for i := range profiles {
		if profiles[i].Name == name {
			return profiles[i], true
		}
	}
	return contracts.Profile{}, false
}
