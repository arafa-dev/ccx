package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/cli"
	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/hooks"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestHooksInstallStatusUninstallJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeCLITestFile(t, filepath.Join(cfgDir, "settings.json"), `{
  "theme": "dark",
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "/usr/local/bin/user-stop"}]}
    ]
  }
}`)
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(t, "hooks", "install", "--profile", "work", "--json")
	installResults := decodeHookResults(t, out)
	if len(installResults) != 1 || installResults[0].Profile != "work" || !installResults[0].Installed {
		t.Fatalf("install results = %+v, want installed work", installResults)
	}
	if installResults[0].Status != hooks.StatusInstalled || installResults[0].SettingsPath != filepath.Join(cfgDir, "settings.json") {
		t.Fatalf("install result = %+v, want installed status and settings path", installResults[0])
	}
	if installResults[0].BackupPath == "" {
		t.Fatalf("install backup path empty for existing settings file")
	}

	out = runCLI(t, "hooks", "status", "--profile", "work", "--json")
	statusResults := decodeHookResults(t, out)
	if len(statusResults) != 1 || statusResults[0].Status != hooks.StatusInstalled || !statusResults[0].Installed {
		t.Fatalf("status results = %+v, want installed", statusResults)
	}

	out = runCLI(t, "hooks", "uninstall", "--profile", "work", "--json")
	uninstallResults := decodeHookResults(t, out)
	if len(uninstallResults) != 1 || uninstallResults[0].Profile != "work" || uninstallResults[0].Installed {
		t.Fatalf("uninstall results = %+v, want not installed work", uninstallResults)
	}
	if uninstallResults[0].BackupPath == "" {
		t.Fatalf("uninstall backup path empty")
	}
	data, err := os.ReadFile(filepath.Join(cfgDir, "settings.json")) //nolint:gosec // test reads a settings file under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/usr/local/bin/user-stop") {
		t.Fatalf("user hook not preserved after uninstall: %s", data)
	}
}

func TestHooksStatusJSONReportsDisabledHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	runCLI(t, "hooks", "install", "--profile", "work", "--json")

	settingsPath := filepath.Join(cfgDir, "settings.json")
	data, err := os.ReadFile(settingsPath) //nolint:gosec // test reads a settings file under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	settings["disableAllHooks"] = json.RawMessage("true")
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "hooks", "status", "--profile", "work", "--json")
	results := decodeHookResults(t, out)
	if len(results) != 1 || results[0].Status != hooks.StatusDisabled || results[0].Installed || !results[0].Disabled {
		t.Fatalf("status results = %+v, want disabled/non-installed", results)
	}
}

func TestHooksInstallRejectsMissingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, stderr, code := runCLIResult([]string{"hooks", "install", "--profile", "missing"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing profile")
	}
	if !strings.Contains(stderr, "missing") {
		t.Fatalf("stderr = %q, want missing profile name", stderr)
	}
}

func TestHooksJSONErrorsIncludeStablePayload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, args := range [][]string{
		{"hooks", "status", "--profile", "missing", "--json"},
		{"hooks", "install", "--profile", "missing", "--json"},
		{"hooks", "uninstall", "--profile", "missing", "--json"},
	} {
		t.Run(strings.Join(args[:2], "-"), func(t *testing.T) {
			stdout, stderr, code := runCLIResult(args)
			if code == 0 {
				t.Fatalf("expected non-zero exit for %v", args)
			}
			if stdout == "" || stdout == "null\n" {
				t.Fatalf("stdout = %q, want stable JSON error payload; stderr=%q", stdout, stderr)
			}
			payload := decodeHookError(t, stdout)
			if payload.Profile != "missing" {
				t.Fatalf("profile = %q, want missing in payload %#v", payload.Profile, payload)
			}
			if payload.Error == "" || !strings.Contains(payload.Error, "missing") {
				t.Fatalf("error = %q, want missing profile error", payload.Error)
			}
			if payload.Message == "" {
				t.Fatalf("message empty in payload %#v", payload)
			}
		})
	}
}

func TestHooksRecordStoresTelemetryWithoutDaemon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	payload := `{
  "session_id": "sess-cli",
  "transcript_path": "/tmp/sess-cli.jsonl",
  "cwd": "/repo",
  "hook_event_name": "StopFailure",
  "error": "rate_limit",
  "error_details": "429 Too Many Requests",
  "trigger": "stop",
  "timestamp": "2026-05-22T10:00:00Z"
}`
	stdout, stderr, code := runCLIResultWithInput([]string{"hooks", "record", "--profile", "work"}, payload)
	if code != 0 {
		t.Fatalf("record exit %d: stdout=%q stderr=%q", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("record stdout = %q, want empty", stdout)
	}

	ccxHome, err := platform.CCXHome()
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewStore(context.Background(), filepath.Join(ccxHome, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessions, err := store.QuerySessions(context.Background(), contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions length = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.Session != "sess-cli" || got.Status != "failed" || got.FailureError != "rate_limit" {
		t.Fatalf("session = %+v, want failed sess-cli rate_limit", got)
	}
	if !got.LastSeenAt.Equal(time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("LastSeenAt = %v, want 2026-05-22T10:00:00Z", got.LastSeenAt)
	}
}

func TestHooksRecordRequiresProfileFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, stderr, code := runCLIResultWithInput(
		[]string{"hooks", "record"},
		`{"session_id":"s1","hook_event_name":"SessionStart"}`,
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing --profile")
	}
	if !strings.Contains(stderr, "--profile") {
		t.Fatalf("stderr = %q, want --profile error", stderr)
	}
}

func TestHooksRecordHiddenFromHelp(t *testing.T) {
	out := runCLI(t, "hooks", "--help")
	if strings.Contains(out, "record") {
		t.Fatalf("hooks help exposes hidden record command:\n%s", out)
	}
}

func decodeHookResults(t *testing.T, out string) []hooks.Result {
	t.Helper()
	var results []hooks.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid hook JSON: %v\n%s", err, out)
	}
	return results
}

func decodeHookError(t *testing.T, out string) hooks.Result {
	t.Helper()
	var result hooks.Result
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid hook error JSON: %v\n%s", err, out)
	}
	return result
}

func writeCLITestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCLIResultWithInput(args []string, stdin string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   args,
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	return stdout.String(), stderr.String(), code
}
