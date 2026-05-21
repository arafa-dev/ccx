package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

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

			if err := ingestAllProfiles(ctx, deps); err != nil {
				return fmt.Errorf("scanning: %w", err)
			}
			profiles, err := deps.Profiles.List(ctx)
			if err != nil {
				return err
			}

			evaluator := headroom.Evaluator{
				Store:   deps.Store,
				Pricing: deps.Pricing,
			}
			result, err := evaluator.Evaluate(ctx, profiles, headroom.Options{IncludeUnavailable: includeUnavailable})
			if err != nil {
				return err
			}
			if result.Recommendation == nil {
				result.Error = "no recommendable profiles"
				if asJSON {
					_ = json.NewEncoder(c.OutOrStdout()).Encode(result)
				}
				return fmt.Errorf("no recommendable profiles")
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
	_, _ = fmt.Fprintln(tw, "PROFILE\tAVAILABLE\tSCORE\tHEADROOM\tAUTH\tREASONS")
	for _, c := range result.Candidates {
		_, _ = fmt.Fprintf(
			tw, "%s\t%t\t%.1f\t%.1f%%\t%s\t%s\n",
			c.Profile,
			c.Available,
			c.Score,
			c.HeadroomPercent,
			c.AuthStatus,
			firstReason(c.Reasons),
		)
	}
	return tw.Flush()
}

func firstReason(reasons []string) string {
	if len(reasons) == 0 {
		return "-"
	}
	return reasons[0]
}
