package quota

// DefaultCaps returns the shipped default per-window turn caps for the given
// Anthropic plan tier. These are best-effort defaults; users may override them
// per profile via ProfileLimits.Caps5hTurns and CapsWeeklyTurns. Unknown tiers
// (including the empty string and "api") return zeros, which disables
// plan-aware quota tracking for that profile in downstream consumers.
//
// Numeric values current as of 2026-05-24; revisit when Anthropic publishes
// authoritative caps (see spec §13).
func DefaultCaps(tier string) (turns5h, turnsWeekly int) {
	switch tier {
	case "pro":
		return 45, 0
	case "max5":
		return 225, 0
	case "max20":
		return 900, 0
	default:
		// "api", "", and unknown tiers all opt out.
		return 0, 0
	}
}
