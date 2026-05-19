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
	Name       string    `json:"name"        toml:"name"`
	ConfigDir  string    `json:"config_dir"  toml:"config_dir"`
	Label      string    `json:"label"       toml:"label,omitempty"`
	Color      string    `json:"color"       toml:"color,omitempty"`
	CreatedAt  time.Time `json:"created_at"  toml:"created_at"`
	LastUsedAt time.Time `json:"last_used_at" toml:"last_used_at"`
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
