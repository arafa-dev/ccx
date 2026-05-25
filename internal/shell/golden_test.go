package shell_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
)

// update regenerates the golden files when set to true via `-update`.
//
//	go test ./internal/shell/... -update
var update = flag.Bool("update", false, "regenerate golden files under testdata/golden/")

// fixtureProfile is the profile used for every golden-file test. Its name and
// config dir are deliberately benign -- edge-case escaping is exercised by
// dedicated tests in escape_edge_test.go.
var fixtureProfile = contracts.Profile{
	Name:      "work",
	ConfigDir: "/Users/arafa/.claude-profiles/work",
}

func TestEmitUseScriptGolden(t *testing.T) {
	cases := []struct {
		shell  contracts.Shell
		golden string
	}{
		{contracts.ShellZsh, "use_zsh.txt"},
		{contracts.ShellBash, "use_bash.txt"},
		{contracts.ShellFish, "use_fish.txt"},
		{contracts.ShellPowerShell, "use_pwsh.txt"},
	}
	e := shell.New()
	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitUseScript(fixtureProfile, tc.shell)
			if err != nil {
				t.Fatalf("EmitUseScript: %v", err)
			}
			compareGolden(t, tc.golden, got)
		})
	}
}

func TestEmitInitScriptGolden(t *testing.T) {
	cases := []struct {
		shell  contracts.Shell
		golden string
	}{
		{contracts.ShellZsh, "init_zsh.txt"},
		{contracts.ShellBash, "init_bash.txt"},
		{contracts.ShellFish, "init_fish.txt"},
		{contracts.ShellPowerShell, "init_pwsh.txt"},
	}
	e := shell.New()
	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitInitScript(tc.shell)
			if err != nil {
				t.Fatalf("EmitInitScript: %v", err)
			}
			compareGolden(t, tc.golden, got)
		})
	}
}

func TestEmitClaudeWrapperGolden(t *testing.T) {
	cases := []struct {
		name   string
		got    string
		golden string
	}{
		{"posix", shell.EmitClaudeWrapperPosix(), "claude_wrapper_posix.txt"},
		{"fish", shell.EmitClaudeWrapperFish(), "claude_wrapper_fish.txt"},
		{"pwsh", shell.EmitClaudeWrapperPowerShell(), "claude_wrapper_pwsh.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compareGolden(t, tc.golden, tc.got)
		})
	}
}

func TestEmitInitScriptWithClaudeGolden(t *testing.T) {
	cases := []struct {
		shell         contracts.Shell
		initGolden    string
		wrapperGolden string
	}{
		{contracts.ShellZsh, "init_zsh.txt", "claude_wrapper_posix.txt"},
		{contracts.ShellBash, "init_bash.txt", "claude_wrapper_posix.txt"},
		{contracts.ShellFish, "init_fish.txt", "claude_wrapper_fish.txt"},
		{contracts.ShellPowerShell, "init_pwsh.txt", "claude_wrapper_pwsh.txt"},
	}
	e := shell.New()
	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitInitScriptWithClaude(tc.shell)
			if err != nil {
				t.Fatalf("EmitInitScriptWithClaude: %v", err)
			}
			want := readGolden(t, tc.initGolden) + "\n" + readGolden(t, tc.wrapperGolden)
			if got != want {
				t.Errorf("combined init mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", tc.shell.String(), want, got)
			}
		})
	}
}

func TestEmitInitScriptWithClaude_UnknownShell(t *testing.T) {
	e := shell.New()

	_, err := e.EmitInitScriptWithClaude(contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error for ShellUnknown, got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}

	_, err = e.EmitInitScriptWithClaude(contracts.Shell(999))
	if err == nil {
		t.Fatal("expected error for Shell(999), got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}
}

// compareGolden reads testdata/golden/<name> and compares it to got. When the
// -update flag is set, the golden is rewritten instead. A missing golden is
// only acceptable when -update is set.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	// Golden names are hard-coded by the test tables above.
	//nolint:gosec
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./internal/shell/... -update` to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	// Golden names are hard-coded by the test tables above.
	//nolint:gosec
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return string(want)
}
