// Package quota computes per-profile plan-aware quota windows from local hook
// telemetry. It owns the shipped default caps per Anthropic subscription tier
// and the math for rolling-5h and rolling-or-anchored-weekly windows.
package quota
