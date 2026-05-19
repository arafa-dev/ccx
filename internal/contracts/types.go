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
