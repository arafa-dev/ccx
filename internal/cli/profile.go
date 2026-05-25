package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/spf13/cobra"
)

func newProfileCommand(_ *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage profiles",
	}
	cmd.AddCommand(
		newProfileAddCmd(),
		newProfileSetCmd(),
		newProfileListCmd(),
		newProfileRmCmd(),
		newProfileCurrentCmd(),
	)
	return cmd
}

func newProfileSetCmd() *cobra.Command {
	var (
		label, color, suggestions, rateLimitCooldown string
		planTier                                     string
		dailyTokens, weeklyTokens, priority          int
		monthlyUSD                                   float64
		asJSON                                       bool
	)
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Update profile limits and suggestion settings",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			name := args[0]
			flags := c.Flags()
			if err := deps.Profiles.Update(ctx, name, func(p *contracts.Profile) error {
				if flags.Changed("label") {
					p.Label = label
				}
				if flags.Changed("color") {
					p.Color = color
				}
				if flags.Changed("daily-tokens") {
					p.Limits.DailyTokenBudget = dailyTokens
				}
				if flags.Changed("weekly-tokens") {
					p.Limits.WeeklyTokenBudget = weeklyTokens
				}
				if flags.Changed("monthly-usd") {
					p.Limits.MonthlyUSDBudget = monthlyUSD
				}
				if flags.Changed("priority") {
					p.Limits.Priority = priority
				}
				if flags.Changed("rate-limit-cooldown") {
					p.Limits.RateLimitCooldown = rateLimitCooldown
				}
				if flags.Changed("plan-tier") {
					tier, err := parsePlanTier(planTier)
					if err != nil {
						return err
					}
					p.Limits.PlanTier = tier
				}
				if flags.Changed("suggestions") {
					enabled, err := parseSuggestionState(suggestions)
					if err != nil {
						return err
					}
					p.Limits.SuggestEnabled = &enabled
				}
				return nil
			}); err != nil {
				return err
			}

			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]string{
					"profile": name,
					"status":  "updated",
				})
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "Profile '%s' updated.\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "human-readable label; pass an empty value to clear")
	cmd.Flags().StringVar(&color, "color", "", "hex accent color; pass an empty value to clear")
	cmd.Flags().IntVar(&dailyTokens, "daily-tokens", 0, "daily token budget; 0 clears the limit")
	cmd.Flags().IntVar(&weeklyTokens, "weekly-tokens", 0, "weekly token budget; 0 clears the limit")
	cmd.Flags().Float64Var(&monthlyUSD, "monthly-usd", 0, "monthly USD budget; 0 clears the limit")
	cmd.Flags().IntVar(&priority, "priority", 0, "suggestion priority")
	cmd.Flags().StringVar(&suggestions, "suggestions", "", "suggestion eligibility: enabled or disabled")
	cmd.Flags().StringVar(&rateLimitCooldown, "rate-limit-cooldown", "", "rate-limit cooldown duration; pass an empty value to clear")
	cmd.Flags().StringVar(&planTier, "plan-tier", "", "subscription plan tier: pro, max5, max20, api, or empty to clear")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func parsePlanTier(value string) (string, error) {
	tier := strings.ToLower(strings.TrimSpace(value))
	switch tier {
	case "", "pro", "max5", "max20", "api":
		return tier, nil
	default:
		return "", fmt.Errorf("--plan-tier must be one of: pro, max5, max20, api")
	}
}

func parseSuggestionState(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "enabled":
		return true, nil
	case "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("--suggestions must be enabled or disabled")
	}
}

func newProfileAddCmd() *cobra.Command {
	var configDir, label, color string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			name := args[0]
			p := contracts.Profile{
				Name:       name,
				ConfigDir:  configDir,
				Label:      label,
				Color:      color,
				CreatedAt:  time.Now().UTC(),
				LastUsedAt: time.Time{},
			}
			if err := deps.Profiles.Add(ctx, p); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(),
				"Profile '%s' added.\nNext: eval \"$(ccx use %s)\" && claude /login\n",
				name, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&configDir, "config-dir", "", "absolute path to the Claude Code config directory (required)")
	cmd.Flags().StringVar(&label, "label", "", "human-readable label")
	cmd.Flags().StringVar(&color, "color", "", "hex accent color for the dashboard, e.g. #3B82F6")
	_ = cmd.MarkFlagRequired("config-dir")
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered profiles with today's usage",
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
				_, _ = fmt.Fprintln(c.OutOrStdout(), "No profiles registered. Run `ccx profile add <name> --config-dir <path>`.")
				return nil
			}
			active, okActive, err := deps.Profiles.Active(ctx)
			if err != nil && !errors.Is(err, contracts.ErrNoActiveProfile) {
				return err
			}
			w := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tCONFIG_DIR\tLAST USED\tTODAY ($)")
			for i := range profiles {
				p := &profiles[i]
				marker := " "
				if okActive && p.Name == active.Name {
					marker = "*"
				}
				today, err := todayCostFor(ctx, deps, p.Name)
				if err != nil {
					return fmt.Errorf("today cost for profile %q: %w", p.Name, err)
				}
				lastUsed := "-"
				if !p.LastUsedAt.IsZero() {
					lastUsed = p.LastUsedAt.Format(time.RFC3339)
				}
				_, _ = fmt.Fprintf(w, "%s%s\t%s\t%s\t$%.2f\n", marker, p.Name, p.ConfigDir, lastUsed, today)
			}
			return w.Flush()
		},
	}
}

func newProfileRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Unregister a profile without deleting its config dir",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()
			if err := deps.Profiles.Remove(ctx, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "Profile '%s' removed.\n", args[0])
			return nil
		},
	}
}

func newProfileCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the active profile",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()
			p, ok, err := deps.Profiles.Active(ctx)
			if err != nil && !errors.Is(err, contracts.ErrNoActiveProfile) {
				return err
			}
			if !ok {
				if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "unmanaged config: %s\n", cfg)
					return nil
				}
				_, _ = fmt.Fprintln(c.OutOrStdout(), "default profile (no CCX_ACTIVE_PROFILE set)")
				return nil
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%s\nconfig: %s\n", p.Name, p.ConfigDir)
			return nil
		},
	}
}

func todayCostFor(ctx context.Context, deps *Deps, name string) (float64, error) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	rows, err := deps.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: name,
		Range:   contracts.TimeRange{Start: start, End: start.Add(24 * time.Hour)},
	})
	if err != nil {
		return 0, err
	}
	var sum float64
	for _, r := range rows {
		c, err := deps.Pricing.Cost(r.Model, r.Day, r.Usage)
		if err != nil {
			return 0, err
		}
		sum += c
	}
	return sum, nil
}
