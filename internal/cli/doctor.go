package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose your install",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()
			d := doctor.New(doctor.Deps{Profiles: deps.Profiles})
			checks, err := d.Run(ctx)
			if err != nil {
				return err
			}
			for _, ch := range checks {
				label := "[OK]"
				switch ch.Status {
				case "warn":
					label = "[WARN]"
				case "fail":
					label = "[FAIL]"
				}
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%s %s - %s\n", label, ch.Name, ch.Detail)
				if ch.Remediation != "" && ch.Status != "ok" {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "   -> %s\n", ch.Remediation)
				}
			}
			return nil
		},
	}
}
