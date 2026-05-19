// Package scanner walks a profile's JSONL session files and emits parsed
// contracts.Events. It is defensive: unknown event types and malformed lines
// are logged and skipped, never panicked on.
// See docs/superpowers/plans (plan A2) for the implementation plan.
package scanner
