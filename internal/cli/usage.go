package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/quotawire"
	"github.com/arafa-dev/ccx/internal/storage"
	"github.com/spf13/cobra"
)

func newUsageCommand(_ *Options) *cobra.Command {
	var (
		profileFlag string
		since       string
		asJSON      bool
		showQuota   bool
	)
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show aggregated token usage and estimated cost",
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
			if showQuota {
				return renderUsageQuota(ctx, deps, c.OutOrStdout(), c.ErrOrStderr(), profileFlag)
			}

			window, err := parseSince(since)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			rows, err := deps.Store.QueryUsage(ctx, contracts.UsageQuery{
				Profile: profileFlag,
				Range:   contracts.TimeRange{Start: now.Add(-window), End: now},
			})
			if err != nil {
				return err
			}

			total, pricingWarnings := priceUsageRows(rows, deps.Pricing)
			writePricingWarnings(c.ErrOrStderr(), pricingWarnings)

			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{
					"rows":  rows,
					"total": total,
				})
			}
			return renderUsageTable(c.OutOrStdout(), rows, total, window)
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "limit to one profile (default: all)")
	cmd.Flags().StringVar(&since, "since", "24h", "lookback window (e.g. 1d, 7d, 30d)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	cmd.Flags().BoolVar(&showQuota, "quota", false, "show plan-aware quota windows alongside token usage")
	return cmd
}

func priceUsageRows(rows []contracts.UsageRow, pricing contracts.PricingTable) (total float64, warnings []string) {
	for i, r := range rows {
		cost, err := pricing.Cost(r.Model, r.Day, r.Usage)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"profile %q project %q model %q day %s: %v",
				r.Profile,
				r.Project,
				r.Model,
				r.Day.Format("2006-01-02"),
				err,
			))
			cost = 0
		}
		rows[i].EstimatedUSD = cost
		total += cost
	}
	return total, warnings
}

func writePricingWarnings(w io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "Warning: pricing lookup failed for %d usage row(s); costs defaulted to $0.00.\n", len(warnings))
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(w, "  - %s\n", warning)
	}
}

func parseSince(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("unrecognized --since value %q", s)
}

func ingestAllProfiles(ctx context.Context, deps *Deps) error {
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		return err
	}
	return ingestSharedAwareProfiles(ctx, deps, profiles)
}

func renderUsageTable(w io.Writer, rows []contracts.UsageRow, total float64, window time.Duration) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "Usage for last %s, all profiles\n\n", window)
	_, _ = fmt.Fprintln(tw, "PROFILE\tTOKENS (in/out/cache)\tEST. COST\tTOP PROJECT")
	type agg struct {
		usage   contracts.Usage
		cost    float64
		top     string
		topCost float64
	}
	per := map[string]*agg{}
	for _, r := range rows {
		a, ok := per[r.Profile]
		if !ok {
			a = &agg{}
			per[r.Profile] = a
		}
		a.usage = a.usage.Add(r.Usage)
		a.cost += r.EstimatedUSD
		if r.EstimatedUSD > a.topCost {
			a.topCost = r.EstimatedUSD
			a.top = r.Project
		}
	}
	names := make([]string, 0, len(per))
	for name := range per {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		a := per[name]
		if a.top == "" {
			a.top = "-"
		}
		_, _ = fmt.Fprintf(
			tw, "%s\t%s\t$%.2f\t%s\n",
			name,
			humanTokens(a.usage),
			a.cost,
			a.top,
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nTotal: $%.2f\n", total)
	return err
}

func renderUsageQuota(ctx context.Context, deps *Deps, out, errOut io.Writer, profileFilter string) error {
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		return err
	}
	if profileFilter != "" {
		profiles = filterProfiles(profiles, profileFilter)
	}
	quotaProvider, err := usageQuotaProvider(deps)
	if err != nil {
		return err
	}
	quotas, err := quotaProvider.Quota(ctx, profileFilter)
	if err != nil {
		return err
	}
	quotaByProfile := make(map[string]contracts.ProfileQuota, len(quotas))
	for i := range quotas {
		quotaByProfile[quotas[i].Profile] = quotas[i]
	}

	now := time.Now().UTC()
	tokenRows, err := deps.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: profileFilter,
		Range:   contracts.TimeRange{Start: now.Add(-24 * time.Hour), End: now},
	})
	if err != nil {
		return fmt.Errorf("query 24h token usage: %w", err)
	}
	tokens24h := usageByProfile(tokenRows)

	costRows, err := deps.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: profileFilter,
		Range:   contracts.TimeRange{Start: now.Add(-30 * 24 * time.Hour), End: now},
	})
	if err != nil {
		return fmt.Errorf("query 30d usage cost: %w", err)
	}
	_, pricingWarnings := priceUsageRows(costRows, deps.Pricing)
	writePricingWarnings(errOut, pricingWarnings)
	usd30d := costByProfile(costRows)

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROFILE\tPLAN\t5H WINDOW\tWEEKLY WINDOW\tTOKENS 24H\tUSD 30D")
	for i := range profiles {
		p := &profiles[i]
		q := quotaByProfile[p.Name]
		_, _ = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t$%.2f\n",
			p.Name,
			planTierLabel(q.PlanTier),
			formatQuotaWindow(q.Window5h),
			formatQuotaWindow(q.WindowWeekly),
			humanCount(tokens24h[p.Name].TotalTokens()),
			usd30d[p.Name],
		)
	}
	return tw.Flush()
}

func usageQuotaProvider(deps *Deps) (*quotawire.Adapter, error) {
	store, ok := deps.Store.(*storage.Store)
	if !ok {
		return nil, fmt.Errorf("usage quota requires *storage.Store, got %T", deps.Store)
	}
	return &quotawire.Adapter{Store: store, Profiles: deps.Profiles}, nil
}

func filterProfiles(profiles []contracts.Profile, profileFilter string) []contracts.Profile {
	filtered := profiles[:0]
	for i := range profiles {
		if profiles[i].Name == profileFilter {
			filtered = append(filtered, profiles[i])
		}
	}
	return filtered
}

func usageByProfile(rows []contracts.UsageRow) map[string]contracts.Usage {
	out := map[string]contracts.Usage{}
	for _, row := range rows {
		out[row.Profile] = out[row.Profile].Add(row.Usage)
	}
	return out
}

func costByProfile(rows []contracts.UsageRow) map[string]float64 {
	out := map[string]float64{}
	for _, row := range rows {
		out[row.Profile] += row.EstimatedUSD
	}
	return out
}

func planTierLabel(planTier string) string {
	if planTier == "" {
		return "-"
	}
	return planTier
}

func formatQuotaWindow(w contracts.QuotaWindow) string {
	if w.Cap == 0 {
		return "-"
	}
	suffix := ""
	if w.Pct >= 100 {
		suffix = " ⛔"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)%s", w.Used, w.Cap, w.Pct, suffix)
}

func humanTokens(u contracts.Usage) string {
	return fmt.Sprintf(
		"%s / %s / %s",
		humanCount(u.InputTokens),
		humanCount(u.OutputTokens),
		humanCount(u.CacheReadTokens+u.CacheCreateTokens),
	)
}

func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
