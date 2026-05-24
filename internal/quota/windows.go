package quota

import (
	"strings"
	"time"
)

// FiveHourWindow is the rolling duration used for short-term quota windows.
const FiveHourWindow = 5 * time.Hour

// WeekWindow is the rolling duration used for weekly quota windows.
const WeekWindow = 7 * 24 * time.Hour

// Window5hBounds returns the lower and upper bounds for the rolling
// five-hour window ending at now. Storage counts turns where since < ts <=
// until.
func Window5hBounds(now time.Time) (since, until time.Time) {
	return now.Add(-FiveHourWindow), now
}

// WindowWeeklyBounds returns bounds for a rolling weekly window unless anchor
// names a weekday, in which case the lower bound is that weekday's most recent
// UTC midnight. Storage counts turns where since < ts <= until.
func WindowWeeklyBounds(now time.Time, anchor string) (since, until time.Time) {
	weekday, ok := parseWeekday(anchor)
	if !ok {
		return now.Add(-WeekWindow), now
	}

	return mostRecentWeekdayMidnight(now, weekday), now
}

// ResetTime5h returns the time when the oldest event in a five-hour window
// ages out.
func ResetTime5h(oldestInWindow time.Time) time.Time {
	if oldestInWindow.IsZero() {
		return time.Time{}
	}
	return oldestInWindow.Add(FiveHourWindow)
}

// ResetTimeWeekly returns the deterministic reset time for oldestInWindow.
// Rolling, empty, and unknown anchors return oldestInWindow plus WeekWindow.
// Valid weekday anchors return the next anchored UTC midnight based on
// oldestInWindow. Zero oldestInWindow returns zero.
func ResetTimeWeekly(oldestInWindow time.Time, anchor string) time.Time {
	if oldestInWindow.IsZero() {
		return time.Time{}
	}
	if _, ok := parseWeekday(anchor); ok {
		return ResetTimeWeeklyAnchored(oldestInWindow, anchor)
	}

	return oldestInWindow.Add(WeekWindow)
}

// ResetTimeWeeklyAnchored returns the next UTC midnight for the configured
// weekly anchor. Unknown anchors fall back to a rolling week from now.
func ResetTimeWeeklyAnchored(now time.Time, anchor string) time.Time {
	weekday, ok := parseWeekday(anchor)
	if !ok {
		return now.Add(WeekWindow)
	}

	mostRecent := mostRecentWeekdayMidnight(now, weekday)
	return mostRecent.Add(WeekWindow)
}

// Pct returns used divided by cap as a percentage clamped to the inclusive
// range [0, 100]. Non-positive caps return 0.
func Pct(used, quotaCap int) float64 {
	if quotaCap <= 0 {
		return 0
	}

	pct := float64(used) / float64(quotaCap) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}

	return pct
}

func parseWeekday(s string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sunday":
		return time.Sunday, true
	case "monday":
		return time.Monday, true
	case "tuesday":
		return time.Tuesday, true
	case "wednesday":
		return time.Wednesday, true
	case "thursday":
		return time.Thursday, true
	case "friday":
		return time.Friday, true
	case "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

func mostRecentWeekdayMidnight(now time.Time, target time.Weekday) time.Time {
	nowUTC := now.UTC()
	midnight := time.Date(
		nowUTC.Year(),
		nowUTC.Month(),
		nowUTC.Day(),
		0,
		0,
		0,
		0,
		time.UTC,
	)
	daysSinceTarget := (int(midnight.Weekday()) - int(target) + 7) % 7

	return midnight.AddDate(0, 0, -daysSinceTarget)
}
