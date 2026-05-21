package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

var requiredEvents = []string{
	"SessionStart",
	"Stop",
	"StopFailure",
	"SessionEnd",
	"PreCompact",
	"PostCompact",
}

func TestInstallCreatesSettingsForMissingFile(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	svc := testService(profile)

	results, err := svc.Install(ctx, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	if results[0].Profile != "work" || !results[0].Installed || results[0].Status != StatusInstalled {
		t.Fatalf("result = %+v, want installed work", results[0])
	}
	if results[0].SettingsPath != filepath.Join(profile.ConfigDir, "settings.json") {
		t.Errorf("SettingsPath = %q, want settings.json under config dir", results[0].SettingsPath)
	}
	if results[0].BackupPath != "" {
		t.Errorf("BackupPath = %q, want empty for newly-created file", results[0].BackupPath)
	}

	settings := readSettings(t, results[0].SettingsPath)
	for _, event := range requiredEvents {
		groups := hookGroups(t, settings, event)
		if len(groups) != 1 {
			t.Fatalf("%s group count = %d, want 1", event, len(groups))
		}
		group := groups[0]
		if event == "Stop" {
			if _, ok := group["matcher"]; ok {
				t.Errorf("Stop group unexpectedly has matcher: %+v", group)
			}
		} else if got, want := stringValue(t, group["matcher"]), expectedMatcher(event); got != want {
			t.Errorf("%s matcher = %q, want %q", event, got, want)
		}
		hooks := hookHandlers(t, group)
		if len(hooks) != 1 {
			t.Fatalf("%s handler count = %d, want 1", event, len(hooks))
		}
		assertManagedHandler(t, hooks[0], "/bin/ccx-test", "work")
	}
}

func TestInstallPreservesUnrelatedSettingsAndUserHooks(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	path := filepath.Join(profile.ConfigDir, "settings.json")
	initial := `{
  "theme": "dark",
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/user-stop", "args": ["--flag"]}
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/lint"}
        ]
      }
    ]
  }
}`
	writeFile(t, path, initial)
	svc := testService(profile)

	results, err := svc.Install(ctx, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if results[0].BackupPath == "" {
		t.Fatalf("BackupPath empty, want backup for existing settings file")
	}
	if _, err := os.Stat(results[0].BackupPath); err != nil {
		t.Fatalf("stat backup: %v", err)
	}

	settings := readSettings(t, path)
	if got := stringValue(t, settings["theme"]); got != "dark" {
		t.Errorf("theme = %q, want dark", got)
	}
	preToolUse := hookGroups(t, settings, "PreToolUse")
	if len(preToolUse) != 1 || stringValue(t, preToolUse[0]["matcher"]) != "Bash" {
		t.Fatalf("PreToolUse not preserved: %+v", preToolUse)
	}
	stopHandlers := hookHandlers(t, hookGroups(t, settings, "Stop")[0])
	if !hasCommand(stopHandlers, "/usr/local/bin/user-stop") {
		t.Fatalf("user Stop hook was not preserved: %+v", stopHandlers)
	}
	if countManagedWork(stopHandlers) != 1 {
		t.Fatalf("managed Stop handler count = %d, want 1: %+v", countManagedWork(stopHandlers), stopHandlers)
	}
}

func TestInstallIsIdempotentAndForceReplacesManagedHooks(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	svc := testService(profile)

	if _, err := svc.Install(ctx, InstallOptions{}); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if _, err := svc.Install(ctx, InstallOptions{}); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	settings := readSettings(t, filepath.Join(profile.ConfigDir, "settings.json"))
	for _, event := range requiredEvents {
		if got := countManagedWork(allHandlers(t, settings, event)); got != 1 {
			t.Fatalf("%s managed handler count after idempotent install = %d, want 1", event, got)
		}
	}

	svc.BinaryPath = func() (string, error) { return "/opt/ccx-new", nil }
	if _, err := svc.Install(ctx, InstallOptions{}); err != nil {
		t.Fatalf("third Install without force: %v", err)
	}
	settings = readSettings(t, filepath.Join(profile.ConfigDir, "settings.json"))
	for _, event := range requiredEvents {
		managed := managedHandlers(allHandlers(t, settings, event), "work")
		if len(managed) != 1 {
			t.Fatalf("%s managed handler count without force = %d, want 1", event, len(managed))
		}
		if got := stringValue(t, managed[0]["command"]); got != "/bin/ccx-test" {
			t.Fatalf("%s command without force = %q, want original /bin/ccx-test", event, got)
		}
	}

	if _, err := svc.Install(ctx, InstallOptions{Force: true}); err != nil {
		t.Fatalf("forced Install: %v", err)
	}
	settings = readSettings(t, filepath.Join(profile.ConfigDir, "settings.json"))
	for _, event := range requiredEvents {
		managed := managedHandlers(allHandlers(t, settings, event), "work")
		if len(managed) != 1 {
			t.Fatalf("%s managed handler count after force = %d, want 1", event, len(managed))
		}
		if got := stringValue(t, managed[0]["command"]); got != "/opt/ccx-new" {
			t.Fatalf("%s command after force = %q, want /opt/ccx-new", event, got)
		}
	}
}

func TestInstallInvalidJSONFailsWithoutChangingFile(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	path := filepath.Join(profile.ConfigDir, "settings.json")
	writeFile(t, path, `{"hooks":`)
	svc := testService(profile)

	_, err := svc.Install(ctx, InstallOptions{})
	if err == nil {
		t.Fatalf("Install succeeded, want invalid JSON error")
	}
	got, readErr := os.ReadFile(path) //nolint:gosec // test reads a settings file under t.TempDir.
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != `{"hooks":` {
		t.Fatalf("settings changed after invalid JSON: %q", got)
	}
	matches, globErr := filepath.Glob(filepath.Join(profile.ConfigDir, "settings.json.ccx-backup-*"))
	if globErr != nil {
		t.Fatalf("Glob: %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("backup files = %v, want none for invalid JSON", matches)
	}
}

func TestTrailingJSONFailsWithoutChangingFile(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	path := filepath.Join(profile.ConfigDir, "settings.json")
	original := `{"theme":"dark"} garbage`
	writeFile(t, path, original)
	svc := testService(profile)

	if _, err := svc.Install(ctx, InstallOptions{}); err == nil {
		t.Fatalf("Install succeeded, want trailing JSON error")
	}
	assertFileContent(t, path, original)
	assertNoBackups(t, profile.ConfigDir)

	if _, err := svc.Uninstall(ctx, UninstallOptions{}); err == nil {
		t.Fatalf("Uninstall succeeded, want trailing JSON error")
	}
	assertFileContent(t, path, original)
	assertNoBackups(t, profile.ConfigDir)

	results, err := svc.Status(ctx, StatusOptions{})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if len(results) != 1 || results[0].Status != StatusInvalid || results[0].Error == "" {
		t.Fatalf("status results = %+v, want invalid result with error", results)
	}
	assertFileContent(t, path, original)
	assertNoBackups(t, profile.ConfigDir)
}

func TestUninstallRemovesOnlyManagedHooks(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	path := filepath.Join(profile.ConfigDir, "settings.json")
	writeFile(t, path, `{
  "theme": "dark",
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/user-stop"},
          {"type": "command", "command": "/old/ccx", "args": ["hooks", "record", "--profile", "work"], "timeout": 5, "statusMessage": "ccx telemetry"}
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {"type": "command", "command": "/old/ccx", "args": ["hooks", "record", "--profile", "work"], "timeout": 5, "statusMessage": "ccx telemetry"}
        ]
      }
    ]
  }
}`)
	svc := testService(profile)

	results, err := svc.Uninstall(ctx, UninstallOptions{})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(results) != 1 || results[0].BackupPath == "" {
		t.Fatalf("results = %+v, want one result with backup", results)
	}
	settings := readSettings(t, path)
	if got := stringValue(t, settings["theme"]); got != "dark" {
		t.Errorf("theme = %q, want dark", got)
	}
	stopHandlers := hookHandlers(t, hookGroups(t, settings, "Stop")[0])
	if !hasCommand(stopHandlers, "/usr/local/bin/user-stop") {
		t.Fatalf("user Stop hook removed: %+v", stopHandlers)
	}
	if got := countManagedWork(stopHandlers); got != 0 {
		t.Fatalf("managed Stop handlers remain = %d", got)
	}
	if got := countManagedWork(allHandlers(t, settings, "SessionStart")); got != 0 {
		t.Fatalf("managed SessionStart handlers remain = %d", got)
	}
}

func TestUninstallPreservesManagedLookingUserHooks(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	path := filepath.Join(profile.ConfigDir, "settings.json")
	original := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/user-extra", "args": ["hooks", "record", "--profile", "work", "--user-extra"], "timeout": 5, "statusMessage": "ccx telemetry"},
          {"type": "command", "command": "/usr/local/bin/user-no-timeout", "args": ["hooks", "record", "--profile", "work"], "statusMessage": "ccx telemetry"},
          {"type": "command", "command": "/usr/local/bin/user-no-status", "args": ["hooks", "record", "--profile", "work"], "timeout": 5}
        ]
      }
    ]
  }
}`
	writeFile(t, path, original)
	svc := testService(profile)

	statusResults, err := svc.Status(ctx, StatusOptions{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statusResults) != 1 || statusResults[0].Status != StatusPartial || statusResults[0].Installed {
		t.Fatalf("status results = %+v, want partial/not installed", statusResults)
	}

	uninstallResults, err := svc.Uninstall(ctx, UninstallOptions{})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(uninstallResults) != 1 || uninstallResults[0].BackupPath != "" {
		t.Fatalf("uninstall results = %+v, want no backup when preserving user hooks", uninstallResults)
	}
	assertFileContent(t, path, original)
	assertNoBackups(t, profile.ConfigDir)
}

func TestInstallAddsRequiredMatcherWhenManagedHookExistsUnderWrongMatcher(t *testing.T) {
	ctx := context.Background()
	profile := testProfile(t, "work")
	path := filepath.Join(profile.ConfigDir, "settings.json")
	writeFile(t, path, `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "wrong",
        "hooks": [
          {"type": "command", "command": "/old/ccx", "args": ["hooks", "record", "--profile", "work"], "timeout": 5, "statusMessage": "ccx telemetry"}
        ]
      }
    ]
  }
}`)
	svc := testService(profile)

	if _, err := svc.Install(ctx, InstallOptions{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	settings := readSettings(t, path)
	groups := hookGroups(t, settings, "SessionStart")
	var (
		foundWrong    bool
		foundRequired bool
	)
	for _, group := range groups {
		switch stringValue(t, group["matcher"]) {
		case "wrong":
			foundWrong = true
		case "startup|resume|clear|compact":
			foundRequired = countManagedWork(hookHandlers(t, group)) == 1
		}
	}
	if !foundWrong || !foundRequired {
		t.Fatalf("SessionStart groups = %+v, want wrong matcher preserved and required matcher added", groups)
	}
}

func TestStatusReportsMissingPartialInstalledAndInvalid(t *testing.T) {
	ctx := context.Background()
	missing := testProfile(t, "missing")
	partial := testProfile(t, "partial")
	installed := testProfile(t, "installed")
	invalid := testProfile(t, "invalid")

	writeFile(t, filepath.Join(partial.ConfigDir, "settings.json"), `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {"type": "command", "command": "/bin/ccx-test", "args": ["hooks", "record", "--profile", "partial"], "timeout": 5, "statusMessage": "ccx telemetry"}
        ]
      }
    ]
  }
}`)
	writeFile(t, filepath.Join(invalid.ConfigDir, "settings.json"), `{"hooks":`)

	svc := testService(missing, partial, installed, invalid)
	if _, err := svc.Install(ctx, InstallOptions{Profile: "installed"}); err != nil {
		t.Fatalf("Install installed profile: %v", err)
	}

	results, err := svc.Status(ctx, StatusOptions{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := map[string]Status{}
	for _, result := range results {
		got[result.Profile] = result.Status
	}
	want := map[string]Status{
		"missing":   StatusMissing,
		"partial":   StatusPartial,
		"installed": StatusInstalled,
		"invalid":   StatusInvalid,
	}
	for profile, status := range want {
		if got[profile] != status {
			t.Errorf("status[%s] = %q, want %q", profile, got[profile], status)
		}
	}
}

func TestInstallProfileFlagRequiresRegisteredProfile(t *testing.T) {
	ctx := context.Background()
	svc := testService(testProfile(t, "work"))
	_, err := svc.Install(ctx, InstallOptions{Profile: "missing"})
	if err == nil {
		t.Fatalf("Install missing profile succeeded")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q, want missing profile name", err)
	}
}

func testService(profiles ...contracts.Profile) *Service {
	return &Service{
		Profiles:   fakeProfiles{profiles: profiles},
		BinaryPath: func() (string, error) { return "/bin/ccx-test", nil },
		Now: func() time.Time {
			return time.Date(2026, 5, 22, 12, 34, 56, 0, time.UTC)
		},
	}
}

func testProfile(t *testing.T, name string) contracts.Profile {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "claude-"+name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return contracts.Profile{Name: name, ConfigDir: dir}
}

type fakeProfiles struct {
	profiles []contracts.Profile
}

func (f fakeProfiles) List(ctx context.Context) ([]contracts.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]contracts.Profile, len(f.profiles))
	copy(out, f.profiles)
	return out, nil
}

func (f fakeProfiles) Get(ctx context.Context, name string) (contracts.Profile, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Profile{}, err
	}
	for i := range f.profiles {
		if f.profiles[i].Name == name {
			return f.profiles[i], nil
		}
	}
	return contracts.Profile{}, errors.New("profile not found")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path) //nolint:gosec // test reads a settings file under t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}

func assertNoBackups(t *testing.T, configDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(configDir, "settings.json.ccx-backup-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("backup files = %v, want none", matches)
	}
}

func readSettings(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test reads a path supplied by test setup.
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Unmarshal settings: %v\n%s", err, data)
	}
	return settings
}

func hookGroups(t *testing.T, settings map[string]json.RawMessage, event string) []map[string]json.RawMessage {
	t.Helper()
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("Unmarshal hooks: %v", err)
	}
	var groups []map[string]json.RawMessage
	if err := json.Unmarshal(hooks[event], &groups); err != nil {
		t.Fatalf("Unmarshal hooks[%s]: %v", event, err)
	}
	return groups
}

func hookHandlers(t *testing.T, group map[string]json.RawMessage) []map[string]json.RawMessage {
	t.Helper()
	var handlers []map[string]json.RawMessage
	if err := json.Unmarshal(group["hooks"], &handlers); err != nil {
		t.Fatalf("Unmarshal group hooks: %v", err)
	}
	return handlers
}

func allHandlers(t *testing.T, settings map[string]json.RawMessage, event string) []map[string]json.RawMessage {
	t.Helper()
	groups := hookGroups(t, settings, event)
	handlers := make([]map[string]json.RawMessage, 0, len(groups))
	for _, group := range groups {
		handlers = append(handlers, hookHandlers(t, group)...)
	}
	return handlers
}

func assertManagedHandler(t *testing.T, handler map[string]json.RawMessage, binary, profile string) {
	t.Helper()
	if got := stringValue(t, handler["type"]); got != "command" {
		t.Errorf("type = %q, want command", got)
	}
	if got := stringValue(t, handler["command"]); got != binary {
		t.Errorf("command = %q, want %q", got, binary)
	}
	if got := intValue(t, handler["timeout"]); got != 5 {
		t.Errorf("timeout = %d, want 5", got)
	}
	if got := stringValue(t, handler["statusMessage"]); got != "ccx telemetry" {
		t.Errorf("statusMessage = %q, want ccx telemetry", got)
	}
	args := stringSliceValue(t, handler["args"])
	wantArgs := []string{"hooks", "record", "--profile", profile}
	if !slices.Equal(args, wantArgs) {
		t.Errorf("args = %#v, want %#v", args, wantArgs)
	}
}

func managedHandlers(handlers []map[string]json.RawMessage, profile string) []map[string]json.RawMessage {
	var out []map[string]json.RawMessage
	for _, handler := range handlers {
		command := stringValueNoFatal(handler["command"])
		if command == "" || !filepath.IsAbs(command) {
			continue
		}
		if stringValueNoFatal(handler["type"]) != "command" {
			continue
		}
		if intValueNoFatal(handler["timeout"]) != 5 {
			continue
		}
		if stringValueNoFatal(handler["statusMessage"]) != "ccx telemetry" {
			continue
		}
		var args []string
		if err := json.Unmarshal(handler["args"], &args); err != nil {
			continue
		}
		if slices.Equal(args, []string{"hooks", "record", "--profile", profile}) {
			out = append(out, handler)
		}
	}
	return out
}

func countManagedWork(handlers []map[string]json.RawMessage) int {
	return len(managedHandlers(handlers, "work"))
}

func hasCommand(handlers []map[string]json.RawMessage, command string) bool {
	for _, handler := range handlers {
		var got string
		if err := json.Unmarshal(handler["command"], &got); err == nil && got == command {
			return true
		}
	}
	return false
}

func expectedMatcher(event string) string {
	switch event {
	case "SessionStart":
		return "startup|resume|clear|compact"
	case "StopFailure":
		return "rate_limit|authentication_failed|oauth_org_not_allowed|billing_error|invalid_request|model_not_found|server_error|max_output_tokens|unknown"
	case "SessionEnd":
		return "clear|resume|logout|prompt_input_exit|bypass_permissions_disabled|other"
	case "PreCompact", "PostCompact":
		return "manual|auto"
	default:
		return ""
	}
}

func stringValue(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var out string
	if len(raw) == 0 {
		return ""
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal string %s: %v", raw, err)
	}
	return out
}

func stringSliceValue(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal []string %s: %v", raw, err)
	}
	return out
}

func intValue(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var out int
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal int %s: %v", raw, err)
	}
	return out
}

func stringValueNoFatal(raw json.RawMessage) string {
	var out string
	if len(raw) == 0 {
		return ""
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ""
	}
	return out
}

func intValueNoFatal(raw json.RawMessage) int {
	var out int
	if len(raw) == 0 {
		return 0
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0
	}
	return out
}
