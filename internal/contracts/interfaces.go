package contracts

import (
	"context"
	"time"
)

// Scanner walks a profile's JSONL files and emits parsed Events. Implementations
// are expected to use the Store's scan-cursor API (via the host package wiring)
// to avoid re-reading already-consumed bytes.
type Scanner interface {
	// Scan walks all JSONL files under profile.ConfigDir/projects/ and emits
	// Events on the returned channel. The channel is closed when scanning
	// completes or when ctx is cancelled. Errors are logged but do not abort
	// the scan; truly fatal errors return early via the error channel.
	Scan(ctx context.Context, profile Profile) (<-chan Event, <-chan error)
}

// Store persists profiles and events. SQLite is the v0.1 implementation, but
// the interface is deliberately storage-agnostic.
type Store interface {
	// Profile CRUD
	SaveProfile(ctx context.Context, p Profile) error
	GetProfile(ctx context.Context, name string) (Profile, error)
	ListProfiles(ctx context.Context) ([]Profile, error)
	DeleteProfile(ctx context.Context, name string) error

	// Event ingestion. Events come from raw JSONL and do not carry profile
	// context, so callers must pass the profile name explicitly.
	InsertEvents(ctx context.Context, profileName string, events []Event) error

	// Usage queries
	QueryUsage(ctx context.Context, q UsageQuery) ([]UsageRow, error)

	// Scan cursors (for incremental scanning)
	GetCursor(ctx context.Context, profileName, filePath string) (offset int64, inode uint64, err error)
	SetCursor(ctx context.Context, profileName, filePath string, offset int64, inode uint64) error

	// Lifecycle
	Migrate(ctx context.Context) error
	Close() error
}

// PricingTable returns estimated USD cost for a given model + usage at a given
// timestamp. Implementations consult an embedded pricing YAML; users may
// override via ~/.ccx/pricing.yaml.
type PricingTable interface {
	Cost(model string, ts time.Time, usage Usage) (float64, error)
	LastUpdated() time.Time
}

// ShellEmitter generates shell-specific snippets for `ccx use` and `ccx init`.
type ShellEmitter interface {
	// EmitUseScript returns the script that, when eval'd by the user's shell,
	// activates the given profile. The script sets CLAUDE_CONFIG_DIR and
	// CCX_ACTIVE_PROFILE.
	EmitUseScript(profile Profile, shell Shell) (string, error)

	// EmitInitScript returns the rc-file snippet the user pastes into their
	// shell config once. The snippet defines a wrapper function so `ccx use foo`
	// works without `eval`.
	EmitInitScript(shell Shell) (string, error)
}

// Doctor runs diagnostic checks and reports them as a structured slice.
type Doctor interface {
	Run(ctx context.Context) ([]DoctorCheck, error)
}

// DoctorCheck is one diagnostic finding. Status is "ok", "warn", or "fail".
type DoctorCheck struct {
	Name        string
	Status      string
	Detail      string
	Remediation string
}
