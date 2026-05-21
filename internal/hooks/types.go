// Package hooks installs Claude Code hooks and records hook telemetry.
package hooks

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Status describes the ccx-managed hook installation state for one profile.
type Status string

const (
	// StatusMissing means settings.json does not exist for the profile.
	StatusMissing Status = "missing"
	// StatusInvalid means settings.json could not be parsed safely.
	StatusInvalid Status = "invalid"
	// StatusPartial means some or all required ccx hooks are absent.
	StatusPartial Status = "partial"
	// StatusInstalled means all required ccx hooks are present.
	StatusInstalled Status = "installed"
)

// InstallOptions controls hook installation.
type InstallOptions struct {
	Profile string
	Force   bool
}

// UninstallOptions controls hook removal.
type UninstallOptions struct {
	Profile string
}

// StatusOptions controls status checks.
type StatusOptions struct {
	Profile string
}

// RecordOptions controls hook payload recording.
type RecordOptions struct {
	Profile string
	Input   io.Reader
}

// Result is emitted by install, uninstall, and status operations.
type Result struct {
	Profile      string `json:"profile"`
	Installed    bool   `json:"installed"`
	Status       Status `json:"status"`
	SettingsPath string `json:"settings_path"`
	BackupPath   string `json:"backup_path,omitempty"`
	Message      string `json:"message,omitempty"`
	Error        string `json:"error,omitempty"`
}

// RecordResult is emitted after a hook payload is recorded.
type RecordResult struct {
	Profile  string `json:"profile"`
	Session  string `json:"session,omitempty"`
	Event    string `json:"event,omitempty"`
	Recorded bool   `json:"recorded"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ProfileSource resolves registered profiles.
type ProfileSource interface {
	List(ctx context.Context) ([]contracts.Profile, error)
	Get(ctx context.Context, name string) (contracts.Profile, error)
}

// Store persists hook events and their session aggregate.
type Store interface {
	InsertHookEvent(ctx context.Context, profileName string, event contracts.HookEvent) error
	UpsertSessionTelemetry(ctx context.Context, profileName string, event contracts.HookEvent) error
}

// Service owns hooks operations for the CLI.
type Service struct {
	Profiles   ProfileSource
	Store      Store
	BinaryPath func() (string, error)
	Now        func() time.Time
}

func (s *Service) binaryPath() (string, error) {
	if s.BinaryPath != nil {
		return s.BinaryPath()
	}
	return os.Executable()
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type hookSpec struct {
	Event      string
	Matcher    string
	HasMatcher bool
}

var requiredHookSpecs = []hookSpec{
	{Event: "SessionStart", Matcher: "startup|resume|clear|compact", HasMatcher: true},
	{Event: "Stop"},
	{Event: "StopFailure", Matcher: "rate_limit|authentication_failed|oauth_org_not_allowed|billing_error|invalid_request|model_not_found|server_error|max_output_tokens|unknown", HasMatcher: true},
	{Event: "SessionEnd", Matcher: "clear|resume|logout|prompt_input_exit|bypass_permissions_disabled|other", HasMatcher: true},
	{Event: "PreCompact", Matcher: "manual|auto", HasMatcher: true},
	{Event: "PostCompact", Matcher: "manual|auto", HasMatcher: true},
}
