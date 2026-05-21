// Package contracts defines the shared types and interfaces used across all
// ccx internal packages. Every other internal/* package imports from this
// package only — never from sibling packages. This isolation is what allows
// Phase 1 development to run in parallel git worktrees.
package contracts

import "time"

// Profile identifies a Claude Code account by its config directory. The
// ConfigDir field is the only thing that determines identity — setting
// CLAUDE_CONFIG_DIR to this value is what isolates the account.
type Profile struct {
	Name       string        `json:"name"         toml:"name"`
	ConfigDir  string        `json:"config_dir"   toml:"config_dir"`
	Label      string        `json:"label"        toml:"label,omitempty"`
	Color      string        `json:"color"        toml:"color,omitempty"`
	CreatedAt  time.Time     `json:"created_at"   toml:"created_at"`
	LastUsedAt time.Time     `json:"last_used_at"  toml:"last_used_at"`
	Limits     ProfileLimits `json:"limits"       toml:"limits,omitempty"`
}

// ProfileLimits configures optional per-profile budget and headroom behavior.
// Zero values mean no explicit limit is configured.
type ProfileLimits struct {
	DailyTokenBudget  int     `json:"daily_token_budget"  toml:"daily_token_budget,omitempty"`
	WeeklyTokenBudget int     `json:"weekly_token_budget" toml:"weekly_token_budget,omitempty"`
	MonthlyUSDBudget  float64 `json:"monthly_usd_budget"  toml:"monthly_usd_budget,omitempty"`
	Priority          int     `json:"priority"            toml:"priority,omitempty"`
	SuggestEnabled    *bool   `json:"suggest_enabled"     toml:"suggest_enabled,omitempty"`
	RateLimitCooldown string  `json:"rate_limit_cooldown" toml:"rate_limit_cooldown,omitempty"`
}

// DaemonStatus is the daemon's externally visible runtime state.
type DaemonStatus struct {
	PID             int       `json:"pid"`
	Version         string    `json:"version"`
	StartedAt       time.Time `json:"started_at"`
	Port            int       `json:"port"`
	URL             string    `json:"url"`
	DBPath          string    `json:"db_path"`
	LogPath         string    `json:"log_path"`
	ProfilesWatched int       `json:"profiles_watched"`
	Running         bool      `json:"running"`
}

// HookEvent captures one daemon-facing hook event emitted by Claude Code.
type HookEvent struct {
	Profile      string    `json:"profile"`
	Session      string    `json:"session"`
	Event        string    `json:"event"`
	Timestamp    time.Time `json:"timestamp"`
	Transcript   string    `json:"transcript"`
	CWD          string    `json:"cwd"`
	Model        string    `json:"model"`
	Source       string    `json:"source"`
	Permission   string    `json:"permission"`
	Reason       string    `json:"reason"`
	Error        string    `json:"error"`
	ErrorDetails string    `json:"error_details"`
	Trigger      string    `json:"trigger"`
}

// SessionTelemetry is the current aggregate state for one Claude Code session.
type SessionTelemetry struct {
	Profile        string    `json:"profile"`
	Session        string    `json:"session"`
	Transcript     string    `json:"transcript"`
	CWD            string    `json:"cwd"`
	Model          string    `json:"model"`
	Source         string    `json:"source"`
	Permission     string    `json:"permission"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        time.Time `json:"ended_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	Status         string    `json:"status"`
	EndReason      string    `json:"end_reason"`
	FailureError   string    `json:"failure_error"`
	FailureDetails string    `json:"failure_details"`
	CompactCount   int       `json:"compact_count"`
}

// ProfileHealth records the latest authentication health check for a profile.
type ProfileHealth struct {
	Profile    string    `json:"profile"`
	CheckedAt  time.Time `json:"checked_at"`
	AuthStatus string    `json:"auth_status"`
	AuthDetail string    `json:"auth_detail"`
}

// HeadroomRecommendation is an advisory ranking for profile selection.
type HeadroomRecommendation struct {
	Profile         string    `json:"profile"`
	Score           float64   `json:"score"`
	HeadroomPercent float64   `json:"headroom_percent"`
	Available       bool      `json:"available"`
	Reason          string    `json:"reason"`
	CooldownUntil   time.Time `json:"cooldown_until"`
	AuthStatus      string    `json:"auth_status"`
}

// SessionQuery filters session telemetry rows.
type SessionQuery struct {
	Profile string    `json:"profile"`
	Status  string    `json:"status"`
	Since   time.Time `json:"since"`
	Limit   int       `json:"limit"`
}

// Usage holds the token counts for a single Claude Code event. All fields are
// non-negative. Token counts come from the upstream JSONL `message.usage` block.
type Usage struct {
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	CacheReadTokens   int `json:"cache_read_tokens"`
	CacheCreateTokens int `json:"cache_create_tokens"`
}

// Add returns the element-wise sum of u and v.
func (u Usage) Add(v Usage) Usage {
	return Usage{
		InputTokens:       u.InputTokens + v.InputTokens,
		OutputTokens:      u.OutputTokens + v.OutputTokens,
		CacheReadTokens:   u.CacheReadTokens + v.CacheReadTokens,
		CacheCreateTokens: u.CacheCreateTokens + v.CacheCreateTokens,
	}
}

// TotalTokens returns the sum of all four token counts. Useful for one-number
// usage displays, but cost calculations should use the per-bucket fields
// because each bucket has a different rate.
func (u Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheCreateTokens
}

// Event is a single parsed JSONL line from a Claude Code session file.
// The Usage field is non-nil only for assistant events that carry token counts.
type Event struct {
	UUID      string    `json:"uuid"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Project   string    `json:"project"`
	Model     string    `json:"model,omitempty"`
	Usage     *Usage    `json:"usage,omitempty"`
}

// TimeRange is a closed interval [Start, End] used for usage queries.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// Contains reports whether t falls within the closed interval [Start, End].
func (r TimeRange) Contains(t time.Time) bool {
	return !t.Before(r.Start) && !t.After(r.End)
}

// UsageQuery filters and groups events for the Store.QueryUsage method.
// An empty Profile means "all profiles." An empty Project means "all projects."
type UsageQuery struct {
	Profile string
	Project string
	Range   TimeRange
}

// UsageRow is one aggregated row returned by Store.QueryUsage. Aggregation
// granularity (per-profile, per-day, per-project) is determined by the
// concrete Store implementation.
type UsageRow struct {
	Profile      string    `json:"profile"`
	Project      string    `json:"project,omitempty"`
	Model        string    `json:"model,omitempty"`
	Day          time.Time `json:"day"` // truncated to start of day in UTC
	Usage        Usage     `json:"usage"`
	SessionCount int       `json:"session_count"`
	EstimatedUSD float64   `json:"estimated_usd"` // populated by the caller after pricing lookup
}

// Shell identifies a shell flavor for the purpose of emitting init scripts
// and `ccx use` shell-eval output.
type Shell int

const (
	// ShellUnknown represents an unrecognized shell.
	ShellUnknown Shell = iota
	// ShellZsh represents zsh.
	ShellZsh
	// ShellBash represents bash.
	ShellBash
	// ShellFish represents fish.
	ShellFish
	// ShellPowerShell represents PowerShell.
	ShellPowerShell
)

// String returns the canonical name of the shell.
func (s Shell) String() string {
	switch s {
	case ShellZsh:
		return "zsh"
	case ShellBash:
		return "bash"
	case ShellFish:
		return "fish"
	case ShellPowerShell:
		return "pwsh"
	default:
		return "unknown"
	}
}

// ParseShell parses a shell name. Accepts "zsh", "bash", "fish", "pwsh",
// "powershell". Returns (ShellUnknown, false) for unknown input.
func ParseShell(s string) (Shell, bool) {
	switch s {
	case "zsh":
		return ShellZsh, true
	case "bash":
		return ShellBash, true
	case "fish":
		return ShellFish, true
	case "pwsh", "powershell":
		return ShellPowerShell, true
	default:
		return ShellUnknown, false
	}
}
