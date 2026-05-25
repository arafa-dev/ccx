package headroom

// PressureLevel categorizes a profile's quota pressure into one of four bands.
type PressureLevel int

const (
	// PressureNone means the profile is below the warn threshold.
	PressureNone PressureLevel = iota
	// PressureWarn means the profile crossed the warn threshold but is still
	// below the soft-penalty threshold.
	PressureWarn
	// PressureSoft means the profile is between the soft-penalty and hard
	// thresholds. Score is reduced by SoftPenalty.
	PressureSoft
	// PressureHard means the profile is at or above the hard cap.
	PressureHard
)

const (
	// ThresholdWarnPct is the pressure percentage where warning starts.
	ThresholdWarnPct = 75.0
	// ThresholdSoftPct is the pressure percentage where scoring penalty starts.
	ThresholdSoftPct = 90.0
	// ThresholdHardPct is the pressure percentage where a profile is capped.
	ThresholdHardPct = 100.0
)

// SoftPenaltyMax is the maximum score penalty applied in the soft band.
const SoftPenaltyMax = 20.0

// PressureLevelFromPct returns the band a pressure percentage falls into.
func PressureLevelFromPct(pct float64) PressureLevel {
	switch {
	case pct >= ThresholdHardPct:
		return PressureHard
	case pct >= ThresholdSoftPct:
		return PressureSoft
	case pct >= ThresholdWarnPct:
		return PressureWarn
	default:
		return PressureNone
	}
}

// SoftPenalty returns the linear score penalty for the soft band.
func SoftPenalty(pct float64) float64 {
	if pct < ThresholdSoftPct {
		return 0
	}
	if pct >= ThresholdHardPct {
		return SoftPenaltyMax
	}
	p := (pct - ThresholdSoftPct) * 2
	if p > SoftPenaltyMax {
		return SoftPenaltyMax
	}
	return p
}
