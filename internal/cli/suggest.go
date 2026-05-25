package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"text/tabwriter"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/spf13/cobra"
)

func newSuggestCommand(_ *Options) *cobra.Command {
	var (
		asJSON             bool
		includeUnavailable bool
	)
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "Suggest the profile with the most advisory headroom",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			profiles, err := deps.Profiles.List(ctx)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				return writeSuggestError(c, asJSON, "no profiles registered")
			}

			scanFailures, err := ingestSuggestProfiles(ctx, deps, profiles)
			if err != nil {
				return err
			}

			headroomStore, err := suggestHeadroomStore(deps)
			if err != nil {
				return err
			}
			evaluator := headroom.Evaluator{
				Store:   headroomStore,
				Pricing: deps.Pricing,
			}
			result, err := evaluator.Evaluate(ctx, profiles, headroom.Options{
				IncludeUnavailable: includeUnavailable,
				UnavailableReasons: scanFailures,
			})
			if err != nil {
				return err
			}
			if result.Recommendation == nil {
				return writeSuggestResultError(c, asJSON, result, "no recommendable profiles")
			}
			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(result)
			}
			return renderSuggest(c.OutOrStdout(), result)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	cmd.Flags().BoolVar(&includeUnavailable, "include-unavailable", false, "allow auth-failed profiles to be considered")
	return cmd
}

func ingestSuggestProfiles(ctx context.Context, deps *Deps, profiles []contracts.Profile) (map[string]string, error) {
	failures := make(map[string]string)
	for i := range profiles {
		p := &profiles[i]
		if err := deps.Store.SaveProfile(ctx, *p); err != nil {
			return nil, fmt.Errorf("saving profile %q before scan: %w", p.Name, err)
		}
		if err := ingestSuggestProfile(ctx, deps, p); err != nil {
			failures[p.Name] = fmt.Sprintf("scan failed: %v", err)
		}
	}
	return failures, nil
}

func ingestSuggestProfile(ctx context.Context, deps *Deps, p *contracts.Profile) error {
	events, errs := deps.Scanner.Scan(ctx, *p)
	batch := make([]contracts.Event, 0, 256)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := deps.Store.InsertEvents(ctx, p.Name, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	var scanErr error
	for events != nil || errs != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				events = nil
				if err := flush(); err != nil {
					return err
				}
				continue
			}
			batch = append(batch, ev)
			if len(batch) >= cap(batch) {
				if err := flush(); err != nil {
					return err
				}
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil && scanErr == nil {
				scanErr = err
			}
		}
	}
	return scanErr
}

func writeSuggestError(c *cobra.Command, asJSON bool, message string) error {
	return writeSuggestResultError(c, asJSON, headroom.Result{Candidates: []headroom.Candidate{}}, message)
}

func writeSuggestResultError(c *cobra.Command, asJSON bool, result headroom.Result, message string) error {
	result.Error = message
	if asJSON {
		_ = json.NewEncoder(c.OutOrStdout()).Encode(result)
	}
	return errors.New(message)
}

func renderSuggest(w io.Writer, result headroom.Result) error {
	rec := result.Recommendation
	if rec == nil {
		return fmt.Errorf("no recommendable profiles")
	}
	_, _ = fmt.Fprintf(w, "Recommended profile: %s\n", rec.Profile)
	_, _ = fmt.Fprintf(w, "Score: %.1f  Headroom: %.1f%%  Auth: %s\n", rec.Score, rec.HeadroomPercent, rec.AuthStatus)
	_, _ = fmt.Fprintln(w, "Reasons:")
	for _, reason := range rec.Reasons {
		_, _ = fmt.Fprintf(w, "  - %s\n", reason)
	}
	_, _ = fmt.Fprintf(w, "\nSuggested command: ccx use %s\n\n", rec.Profile)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROFILE\tAVAILABLE\tSCORE\tHEADROOM\t5H\tWEEKLY\tAUTH\tREASONS")
	for i := range result.Candidates {
		c := &result.Candidates[i]
		_, _ = fmt.Fprintf(
			tw, "%s\t%t\t%.1f\t%.1f%%\t%s\t%s\t%s\t%s\n",
			c.Profile,
			c.Available,
			c.Score,
			c.HeadroomPercent,
			formatSuggestQuotaWindow(c.Quota5h),
			formatSuggestQuotaWindow(c.QuotaWeekly),
			c.AuthStatus,
			firstReason(c.Reasons),
		)
	}
	return tw.Flush()
}

func formatSuggestQuotaWindow(w *contracts.QuotaWindow) string {
	if w == nil || w.Cap == 0 {
		return "—"
	}
	suffix := ""
	if w.Pct >= 100 {
		suffix = " ⛔"
	}
	return fmt.Sprintf("%d/%d (%s)%s", w.Used, w.Cap, formatSuggestQuotaPct(w.Pct), suffix)
}

func formatSuggestQuotaPct(pct float64) string {
	if pct >= 100 {
		return "100%"
	}
	pct = math.Floor(pct*10) / 10
	if pct >= 100 {
		pct = 99.9
	}
	return fmt.Sprintf("%.1f%%", pct)
}

func firstReason(reasons []string) string {
	if len(reasons) == 0 {
		return "-"
	}
	return reasons[0]
}
