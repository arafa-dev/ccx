package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

const settingsFileName = "settings.json"

// Install adds ccx-managed Claude Code hook handlers to registered profiles.
func (s *Service) Install(ctx context.Context, opts InstallOptions) ([]Result, error) {
	profiles, err := s.targetProfiles(ctx, opts.Profile)
	if err != nil {
		return nil, err
	}
	binary, err := s.binaryPath()
	if err != nil {
		return nil, fmt.Errorf("ccx executable path: %w", err)
	}

	results := make([]Result, 0, len(profiles))
	for i := range profiles {
		result, err := s.installProfile(&profiles[i], binary, opts.Force)
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// Uninstall removes only ccx-managed hook handlers from registered profiles.
func (s *Service) Uninstall(ctx context.Context, opts UninstallOptions) ([]Result, error) {
	profiles, err := s.targetProfiles(ctx, opts.Profile)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(profiles))
	for i := range profiles {
		result, err := s.uninstallProfile(&profiles[i])
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// Status reports hook installation status for registered profiles.
func (s *Service) Status(ctx context.Context, opts StatusOptions) ([]Result, error) {
	profiles, err := s.targetProfiles(ctx, opts.Profile)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(profiles))
	for i := range profiles {
		results = append(results, s.statusProfile(&profiles[i]))
	}
	return results, nil
}

func (s *Service) targetProfiles(ctx context.Context, profileName string) ([]contracts.Profile, error) {
	if s.Profiles == nil {
		return nil, errors.New("hooks: profile registry is nil")
	}
	if profileName != "" {
		p, err := s.Profiles.Get(ctx, profileName)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", profileName, err)
		}
		return []contracts.Profile{p}, nil
	}
	profiles, err := s.Profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func (s *Service) installProfile(profile *contracts.Profile, binary string, force bool) (Result, error) {
	path := profileSettingsPath(profile)
	result := Result{
		Profile:      profile.Name,
		SettingsPath: path,
	}

	settings, existed, err := loadSettings(path)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result, err
	}

	changed, err := installManagedHooks(settings, profile.Name, binary, force)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result, err
	}
	if changed {
		backup, err := writeSettings(path, settings, existed, s.now())
		if err != nil {
			result.Error = err.Error()
			return result, err
		}
		result.BackupPath = backup
	}

	status, disabled, err := statusForSettings(settings, profile.Name)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result, err
	}
	result.Status = status
	result.Disabled = disabled
	result.Installed = status == StatusInstalled
	if result.Installed {
		result.Message = "ccx hooks installed"
	}
	return result, nil
}

func (s *Service) uninstallProfile(profile *contracts.Profile) (Result, error) {
	path := profileSettingsPath(profile)
	result := Result{
		Profile:      profile.Name,
		SettingsPath: path,
	}

	settings, existed, err := loadSettings(path)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result, err
	}
	if !existed {
		result.Status = StatusMissing
		result.Message = "settings.json not found"
		return result, nil
	}

	changed, err := removeManagedHooks(settings, profile.Name)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result, err
	}
	if changed {
		backup, err := writeSettings(path, settings, true, s.now())
		if err != nil {
			result.Error = err.Error()
			return result, err
		}
		result.BackupPath = backup
		result.Message = "ccx hooks uninstalled"
	} else {
		result.Message = "no ccx hooks installed"
	}

	status, disabled, err := statusForSettings(settings, profile.Name)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result, err
	}
	result.Status = status
	result.Disabled = disabled
	result.Installed = status == StatusInstalled
	return result, nil
}

func (s *Service) statusProfile(profile *contracts.Profile) Result {
	path := profileSettingsPath(profile)
	result := Result{
		Profile:      profile.Name,
		SettingsPath: path,
	}
	settings, existed, err := loadSettings(path)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result
	}
	if !existed {
		result.Status = StatusMissing
		result.Message = "settings.json not found"
		return result
	}
	status, disabled, err := statusForSettings(settings, profile.Name)
	if err != nil {
		result.Status = StatusInvalid
		result.Error = err.Error()
		return result
	}
	result.Status = status
	result.Disabled = disabled
	result.Installed = result.Status == StatusInstalled
	return result
}

func profileSettingsPath(profile *contracts.Profile) string {
	return filepath.Join(profile.ConfigDir, settingsFileName)
}

func loadSettings(path string) (settings map[string]any, existed bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from a registered profile config dir.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, false, nil
		}
		return nil, false, fmt.Errorf("reading settings %q: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&settings); err != nil {
		return nil, true, fmt.Errorf("parsing settings %q: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, true, fmt.Errorf("parsing settings %q: trailing JSON data", path)
		}
		return nil, true, fmt.Errorf("parsing settings %q: %w", path, err)
	}
	if settings == nil {
		return nil, true, fmt.Errorf("parsing settings %q: root must be a JSON object", path)
	}
	return settings, true, nil
}

func installManagedHooks(settings map[string]any, profile, binary string, force bool) (bool, error) {
	hooks, err := hooksObject(settings, true)
	if err != nil {
		return false, err
	}
	changed := false
	for _, spec := range requiredHookSpecs {
		groups, err := eventGroups(hooks, spec.Event, true)
		if err != nil {
			return false, err
		}
		if force {
			var removed bool
			groups, removed, err = removeManagedFromGroups(groups, profile)
			if err != nil {
				return false, err
			}
			if removed {
				hooks[spec.Event] = groups
				changed = true
			}
		}
		if hasManagedHandlerInRequiredGroup(groups, spec, profile) {
			continue
		}
		groups, err = addManagedHandler(groups, spec, managedHook(binary, profile))
		if err != nil {
			return false, err
		}
		hooks[spec.Event] = groups
		changed = true
	}
	return changed, nil
}

func removeManagedHooks(settings map[string]any, profile string) (bool, error) {
	hooks, err := hooksObject(settings, false)
	if err != nil {
		return false, err
	}
	if hooks == nil {
		return false, nil
	}

	changed := false
	for _, spec := range requiredHookSpecs {
		groups, err := eventGroups(hooks, spec.Event, false)
		if err != nil {
			return false, err
		}
		if groups == nil {
			continue
		}
		next, removed, err := removeManagedFromGroups(groups, profile)
		if err != nil {
			return false, err
		}
		if removed {
			hooks[spec.Event] = next
			changed = true
		}
	}
	return changed, nil
}

func statusForSettings(settings map[string]any, profile string) (Status, bool, error) {
	if disabled, ok := boolFromAny(settings["disableAllHooks"]); ok && disabled {
		return StatusDisabled, true, nil
	}
	hooks, err := hooksObject(settings, false)
	if err != nil {
		return StatusInvalid, false, err
	}
	if hooks == nil {
		return StatusPartial, false, nil
	}
	installed := 0
	for _, spec := range requiredHookSpecs {
		groups, err := eventGroups(hooks, spec.Event, false)
		if err != nil {
			return StatusInvalid, false, err
		}
		if groups == nil {
			continue
		}
		if hasManagedHandlerInRequiredGroup(groups, spec, profile) {
			installed++
		}
	}
	if installed == len(requiredHookSpecs) {
		return StatusInstalled, false, nil
	}
	return StatusPartial, false, nil
}

func hooksObject(settings map[string]any, create bool) (map[string]any, error) {
	raw, ok := settings["hooks"]
	if !ok {
		if !create {
			return nil, nil
		}
		hooks := map[string]any{}
		settings["hooks"] = hooks
		return hooks, nil
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("settings hooks must be a JSON object")
	}
	return hooks, nil
}

func eventGroups(hooks map[string]any, event string, create bool) ([]any, error) {
	raw, ok := hooks[event]
	if !ok {
		if !create {
			return nil, nil
		}
		groups := []any{}
		hooks[event] = groups
		return groups, nil
	}
	groups, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("hooks.%s must be a JSON array", event)
	}
	for _, rawGroup := range groups {
		if _, ok := rawGroup.(map[string]any); !ok {
			return nil, fmt.Errorf("hooks.%s group must be a JSON object", event)
		}
	}
	return groups, nil
}

func addManagedHandler(groups []any, spec hookSpec, handler map[string]any) ([]any, error) {
	for _, rawGroup := range groups {
		group := rawGroup.(map[string]any)
		if groupMatchesSpec(group, spec) {
			handlers, err := handlersArray(group, true)
			if err != nil {
				return nil, err
			}
			group["hooks"] = append(handlers, handler)
			return groups, nil
		}
	}
	group := map[string]any{
		"hooks": []any{handler},
	}
	if spec.HasMatcher {
		group["matcher"] = spec.Matcher
	}
	return append(groups, group), nil
}

func removeManagedFromGroups(groups []any, profile string) (nextGroups []any, removed bool, err error) {
	nextGroups = groups
	for _, rawGroup := range nextGroups {
		group := rawGroup.(map[string]any)
		handlers, err := handlersArray(group, false)
		if err != nil {
			return nil, false, err
		}
		if handlers == nil {
			continue
		}
		next := handlers[:0]
		for _, rawHandler := range handlers {
			handler, ok := rawHandler.(map[string]any)
			if ok && isManagedHandler(handler, profile) {
				removed = true
				continue
			}
			next = append(next, rawHandler)
		}
		group["hooks"] = next
	}
	return nextGroups, removed, nil
}

func handlersArray(group map[string]any, create bool) ([]any, error) {
	raw, ok := group["hooks"]
	if !ok {
		if !create {
			return nil, nil
		}
		handlers := []any{}
		group["hooks"] = handlers
		return handlers, nil
	}
	handlers, ok := raw.([]any)
	if !ok {
		return nil, errors.New("hook group hooks must be a JSON array")
	}
	return handlers, nil
}

func hasManagedHandlerInRequiredGroup(groups []any, spec hookSpec, profile string) bool {
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok || !groupMatchesSpec(group, spec) {
			continue
		}
		handlers, err := handlersArray(group, false)
		if err != nil {
			continue
		}
		for _, rawHandler := range handlers {
			handler, ok := rawHandler.(map[string]any)
			if ok && isManagedHandler(handler, profile) {
				return true
			}
		}
	}
	return false
}

func groupMatchesSpec(group map[string]any, spec hookSpec) bool {
	matcher, ok := stringFromAny(group["matcher"])
	if !spec.HasMatcher {
		return !ok || matcher == ""
	}
	return ok && matcher == spec.Matcher
}

func managedHook(binary, profile string) map[string]any {
	return map[string]any{
		"type":          "command",
		"command":       binary,
		"args":          []any{"hooks", "record", "--profile", profile},
		"timeout":       5,
		"statusMessage": "ccx telemetry",
	}
}

func isManagedHandler(handler map[string]any, profile string) bool {
	hookType, ok := stringFromAny(handler["type"])
	if !ok || hookType != "command" {
		return false
	}
	command, ok := stringFromAny(handler["command"])
	if !ok || command == "" || !filepath.IsAbs(command) {
		return false
	}
	args, ok := stringsFromAny(handler["args"])
	if !ok || len(args) != 4 {
		return false
	}
	prefix := []string{"hooks", "record", "--profile", profile}
	for i, want := range prefix {
		if args[i] != want {
			return false
		}
	}
	timeout, ok := intFromAny(handler["timeout"])
	if !ok || timeout != 5 {
		return false
	}
	statusMessage, ok := stringFromAny(handler["statusMessage"])
	if !ok || statusMessage != "ccx telemetry" {
		return false
	}
	return true
}

func stringFromAny(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func boolFromAny(v any) (value, ok bool) {
	b, ok := v.(bool)
	return b, ok
}

func stringsFromAny(v any) ([]string, bool) {
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func writeSettings(path string, settings map[string]any, existed bool, now time.Time) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("creating settings dir: %w", err)
	}

	backup := ""
	if existed {
		var err error
		backup, err = backupSettings(path, now)
		if err != nil {
			return "", err
		}
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding settings: %w", err)
	}
	data = append(data, '\n')

	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*") //nolint:gosec // path is derived from a registered profile config dir.
	if err != nil {
		return "", fmt.Errorf("creating tmp settings: %w", err)
	}
	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("writing tmp settings: %w", err)
	}
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("chmod tmp settings: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("closing tmp settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("renaming tmp settings: %w", err)
	}
	cleanup = false
	return backup, nil
}

func backupSettings(path string, now time.Time) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from a registered profile config dir.
	if err != nil {
		return "", fmt.Errorf("reading settings for backup: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat settings for backup: %w", err)
	}
	stamp := now.UTC().Format("20060102T150405.000000000Z")
	base := path + ".ccx-backup-" + stamp
	for i := 0; ; i++ {
		backup := base
		if i > 0 {
			backup = base + "-" + strconv.Itoa(i+1)
		}
		file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()) //nolint:gosec // path is derived from registered profile config dir.
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("creating settings backup: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			_ = os.Remove(backup)
			return "", fmt.Errorf("writing settings backup: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(backup)
			return "", fmt.Errorf("closing settings backup: %w", err)
		}
		return backup, nil
	}
}
