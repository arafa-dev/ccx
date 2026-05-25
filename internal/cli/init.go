package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
	"github.com/spf13/cobra"
)

func newInitCommand(_ *Options) *cobra.Command {
	var withClaudeWrapper bool
	cmd := &cobra.Command{
		Use:   "init <shell>",
		Short: "Print the rc-file snippet for the given shell",
		Long:  "Supported shells: zsh, bash, fish, pwsh",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			sh, ok := contracts.ParseShell(args[0])
			if !ok {
				return fmt.Errorf("%w: %q", contracts.ErrUnknownShell, args[0])
			}
			emitter := shell.New()
			emitInitScript := emitter.EmitInitScript
			if withClaudeWrapper {
				emitInitScript = emitter.EmitInitScriptWithClaude
			}
			script, err := emitInitScript(sh)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(c.OutOrStdout(), script)
			return nil
		},
	}
	cmd.Flags().BoolVar(&withClaudeWrapper, "with-claude-wrapper", false, "additionally emit a claude wrapper that calls ccx run --")
	return cmd
}
