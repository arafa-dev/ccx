package shell_test

import (
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
)

func TestEmitUseScript_NameWithSingleQuote(t *testing.T) {
	p := contracts.Profile{
		Name:      "it's-work",
		ConfigDir: "/Users/arafa/.claude-profiles/work",
	}
	e := shell.New()

	cases := []struct {
		shell contracts.Shell
		want  string
	}{
		{
			contracts.ShellZsh,
			"export CLAUDE_CONFIG_DIR='/Users/arafa/.claude-profiles/work'\n" +
				`export CCX_ACTIVE_PROFILE='it'"'"'s-work'` + "\n",
		},
		{
			contracts.ShellBash,
			"export CLAUDE_CONFIG_DIR='/Users/arafa/.claude-profiles/work'\n" +
				`export CCX_ACTIVE_PROFILE='it'"'"'s-work'` + "\n",
		},
		{
			contracts.ShellFish,
			"set -gx CLAUDE_CONFIG_DIR '/Users/arafa/.claude-profiles/work'\n" +
				`set -gx CCX_ACTIVE_PROFILE 'it'"'"'s-work'` + "\n",
		},
		{
			contracts.ShellPowerShell,
			"$env:CLAUDE_CONFIG_DIR = '/Users/arafa/.claude-profiles/work'\n" +
				"$env:CCX_ACTIVE_PROFILE = 'it''s-work'\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitUseScript(p, tc.shell)
			if err != nil {
				t.Fatalf("EmitUseScript: %v", err)
			}
			if got != tc.want {
				t.Errorf("escape mismatch:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}

// Sanity check: the escaped name must never contain an unbalanced quote run.
// If the escaper is broken, the output below would have one terminal that's
// "open"; counting single quotes (POSIX) or odd-doubled quotes (pwsh) would
// catch it.
func TestEmitUseScript_NameWithSingleQuote_QuoteBalance(t *testing.T) {
	p := contracts.Profile{Name: "a'b'c", ConfigDir: "/x"}
	e := shell.New()
	for _, sh := range []contracts.Shell{contracts.ShellZsh, contracts.ShellBash, contracts.ShellFish} {
		got, err := e.EmitUseScript(p, sh)
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		// POSIX single-quote count must be even.
		if strings.Count(got, "'")%2 != 0 {
			t.Errorf("%s: odd number of single quotes: unbalanced escape: %q", sh, got)
		}
	}
}

func TestEmitUseScript_ConfigDirWithSpacesAndQuote(t *testing.T) {
	p := contracts.Profile{
		Name:      "work",
		ConfigDir: "/Users/arafa/my profiles/it's work",
	}
	e := shell.New()

	cases := []struct {
		shell contracts.Shell
		want  string
	}{
		{
			contracts.ShellZsh,
			`export CLAUDE_CONFIG_DIR='/Users/arafa/my profiles/it'"'"'s work'` + "\n" +
				"export CCX_ACTIVE_PROFILE='work'\n",
		},
		{
			contracts.ShellBash,
			`export CLAUDE_CONFIG_DIR='/Users/arafa/my profiles/it'"'"'s work'` + "\n" +
				"export CCX_ACTIVE_PROFILE='work'\n",
		},
		{
			contracts.ShellFish,
			`set -gx CLAUDE_CONFIG_DIR '/Users/arafa/my profiles/it'"'"'s work'` + "\n" +
				"set -gx CCX_ACTIVE_PROFILE 'work'\n",
		},
		{
			contracts.ShellPowerShell,
			"$env:CLAUDE_CONFIG_DIR = '/Users/arafa/my profiles/it''s work'\n" +
				"$env:CCX_ACTIVE_PROFILE = 'work'\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitUseScript(p, tc.shell)
			if err != nil {
				t.Fatalf("EmitUseScript: %v", err)
			}
			if got != tc.want {
				t.Errorf("escape mismatch:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}
