package shell

import "github.com/arafa-dev/ccx/internal/contracts"

// emitUsePosix returns the export script for zsh and bash.
//
// Format (byte-exact, including trailing newline):
//
//	export CLAUDE_CONFIG_DIR='<escaped-path>'
//	export CCX_ACTIVE_PROFILE='<escaped-name>'
func emitUsePosix(p *contracts.Profile) string {
	return "export CLAUDE_CONFIG_DIR=" + escapePosixSingle(p.ConfigDir) + "\n" +
		"export CCX_ACTIVE_PROFILE=" + escapePosixSingle(p.Name) + "\n"
}

// emitUseFish returns the export script for fish.
//
// Format (byte-exact, including trailing newline):
//
//	set -gx CLAUDE_CONFIG_DIR '<escaped-path>'
//	set -gx CCX_ACTIVE_PROFILE '<escaped-name>'
func emitUseFish(p *contracts.Profile) string {
	return "set -gx CLAUDE_CONFIG_DIR " + escapePosixSingle(p.ConfigDir) + "\n" +
		"set -gx CCX_ACTIVE_PROFILE " + escapePosixSingle(p.Name) + "\n"
}

// emitUsePowerShell returns the export script for PowerShell.
//
// Format (byte-exact, including trailing newline):
//
//	$env:CLAUDE_CONFIG_DIR = '<escaped-path>'
//	$env:CCX_ACTIVE_PROFILE = '<escaped-name>'
func emitUsePowerShell(p *contracts.Profile) string {
	return "$env:CLAUDE_CONFIG_DIR = " + escapePowerShellSingle(p.ConfigDir) + "\n" +
		"$env:CCX_ACTIVE_PROFILE = " + escapePowerShellSingle(p.Name) + "\n"
}

// initPosix is the wrapper function users paste into ~/.zshrc or ~/.bashrc.
// After installation, `ccx use foo` invokes `command ccx use foo` and evals
// the captured stdout. All other ccx subcommands pass through unchanged.
const initPosix = `ccx() {
  if [[ "$1" == "use" ]]; then
    eval "$(command ccx use "${@:2}")"
  else
    command ccx "$@"
  fi
}
`

// initFish is the wrapper function users paste into ~/.config/fish/config.fish.
const initFish = `function ccx
    if test "$argv[1]" = use
        command ccx use $argv[2..] | source
    else
        command ccx $argv
    end
end
`

// initPowerShell is the wrapper function users paste into their PowerShell
// profile (e.g., $PROFILE). It locates the on-disk ccx.exe via Get-Command so
// the function does not recurse into itself when calling out.
const initPowerShell = `function ccx {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    if ($Args.Count -gt 0 -and $Args[0] -eq 'use') {
        $rest = $Args[1..($Args.Count - 1)]
        & (Get-Command ccx.exe).Path use @rest | Out-String | Invoke-Expression
    } else {
        & (Get-Command ccx.exe).Path @Args
    }
}
`

const claudeWrapperPosix = `claude() {
  command ccx run -- "$@"
}
`

// EmitClaudeWrapperPosix returns the zsh/bash snippet that routes `claude`
// invocations through `ccx run`.
func EmitClaudeWrapperPosix() string {
	return claudeWrapperPosix
}

const claudeWrapperFish = `function claude
    command ccx run -- $argv
end
`

// EmitClaudeWrapperFish returns the fish snippet that routes `claude`
// invocations through `ccx run`.
func EmitClaudeWrapperFish() string {
	return claudeWrapperFish
}

const claudeWrapperPowerShell = `function claude {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    $ccx = Get-Command ccx -CommandType Application -ErrorAction SilentlyContinue
    if (-not $ccx) { $ccx = Get-Command ccx.exe -CommandType Application }
    & $ccx.Path run -- @Args
}
`

// EmitClaudeWrapperPowerShell returns the PowerShell snippet that routes
// `claude` invocations through `ccx run`.
func EmitClaudeWrapperPowerShell() string {
	return claudeWrapperPowerShell
}
