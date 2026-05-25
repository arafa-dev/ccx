package shell

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Emitter implements contracts.ShellEmitter. It is stateless and safe for
// concurrent use.
type Emitter struct{}

// New returns a new Emitter.
func New() *Emitter {
	return &Emitter{}
}

// EmitUseScript returns the script that, when eval'd by the user's shell,
// activates the given profile. The script sets CLAUDE_CONFIG_DIR and
// CCX_ACTIVE_PROFILE. Profile name and config-dir path are escaped so that
// embedded quotes and spaces never produce a broken script.
//
// Returns contracts.ErrUnknownShell (wrapped) if sh is not a recognized shell.
func (e *Emitter) EmitUseScript(p contracts.Profile, sh contracts.Shell) (string, error) { //nolint:gocritic // Contract requires Profile by value.
	switch sh {
	case contracts.ShellZsh, contracts.ShellBash:
		return emitUsePosix(&p), nil
	case contracts.ShellFish:
		return emitUseFish(&p), nil
	case contracts.ShellPowerShell:
		return emitUsePowerShell(&p), nil
	case contracts.ShellUnknown:
		return "", fmt.Errorf("emitting use script: %w", contracts.ErrUnknownShell)
	default:
		return "", fmt.Errorf("emitting use script for %q: %w", sh.String(), contracts.ErrUnknownShell)
	}
}

// EmitInitScript returns the rc-file snippet the user pastes into their shell
// config once. The snippet defines a wrapper function so `ccx use foo` works
// without `eval`.
//
// Returns contracts.ErrUnknownShell (wrapped) if sh is not a recognized shell.
func (e *Emitter) EmitInitScript(sh contracts.Shell) (string, error) {
	switch sh {
	case contracts.ShellZsh, contracts.ShellBash:
		return initPosix, nil
	case contracts.ShellFish:
		return initFish, nil
	case contracts.ShellPowerShell:
		return initPowerShell, nil
	case contracts.ShellUnknown:
		return "", fmt.Errorf("emitting init script: %w", contracts.ErrUnknownShell)
	default:
		return "", fmt.Errorf("emitting init script for %q: %w", sh.String(), contracts.ErrUnknownShell)
	}
}

// EmitInitScriptWithClaude returns the rc-file snippet from EmitInitScript plus
// a `claude` wrapper that routes invocations through `ccx run`.
func (e *Emitter) EmitInitScriptWithClaude(sh contracts.Shell) (string, error) {
	initScript, err := e.EmitInitScript(sh)
	if err != nil {
		return "", err
	}

	switch sh {
	case contracts.ShellZsh, contracts.ShellBash:
		return initScript + "\n" + EmitClaudeWrapperPosix(), nil
	case contracts.ShellFish:
		return initScript + "\n" + EmitClaudeWrapperFish(), nil
	case contracts.ShellPowerShell:
		return initScript + "\n" + EmitClaudeWrapperPowerShell(), nil
	case contracts.ShellUnknown:
		return "", fmt.Errorf("emitting init script with claude wrapper: %w", contracts.ErrUnknownShell)
	default:
		return "", fmt.Errorf("emitting init script with claude wrapper for %q: %w", sh.String(), contracts.ErrUnknownShell)
	}
}

// Compile-time check that *Emitter satisfies the contract.
var _ contracts.ShellEmitter = (*Emitter)(nil)
