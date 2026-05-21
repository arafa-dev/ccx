package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Record reads one Claude Code hook payload and stores normalized telemetry.
func (s *Service) Record(ctx context.Context, opts RecordOptions) (RecordResult, error) {
	result := RecordResult{Profile: opts.Profile}
	if opts.Profile == "" {
		err := errors.New("hooks record requires --profile")
		result.Error = err.Error()
		return result, err
	}
	if s.Profiles == nil {
		err := errors.New("hooks: profile registry is nil")
		result.Error = err.Error()
		return result, err
	}
	if s.Store == nil {
		err := errors.New("hooks: store is nil")
		result.Error = err.Error()
		return result, err
	}
	profile, err := s.Profiles.Get(ctx, opts.Profile)
	if err != nil {
		err = fmt.Errorf("profile %q: %w", opts.Profile, err)
		result.Error = err.Error()
		return result, err
	}
	if saver, ok := s.Store.(profileSaver); ok {
		if err := saver.SaveProfile(ctx, profile); err != nil {
			result.Error = err.Error()
			return result, err
		}
	}

	input := opts.Input
	if input == nil {
		input = io.LimitReader(bytes.NewReader(nil), 0)
	}
	event, err := s.parseHookEvent(input, opts.Profile)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if event.Event == "" {
		err := errors.New("hook payload missing hook_event_name")
		result.Error = err.Error()
		return result, err
	}

	if err := s.Store.InsertHookEvent(ctx, opts.Profile, event); err != nil {
		result.Error = err.Error()
		return result, err
	}
	if err := s.Store.UpsertSessionTelemetry(ctx, opts.Profile, event); err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Session = event.Session
	result.Event = event.Event
	result.Recorded = true
	result.Message = "hook telemetry recorded"
	return result, nil
}

type profileSaver interface {
	SaveProfile(ctx context.Context, p contracts.Profile) error
}

func (s *Service) parseHookEvent(r io.Reader, profile string) (contracts.HookEvent, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return contracts.HookEvent{}, fmt.Errorf("reading hook payload: %w", err)
	}
	var payload map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return contracts.HookEvent{}, fmt.Errorf("parsing hook payload: %w", err)
	}

	event := contracts.HookEvent{
		Profile:      profile,
		Session:      payloadString(payload, "session_id"),
		Event:        payloadString(payload, "hook_event_name"),
		Timestamp:    s.payloadTimestamp(payload),
		Transcript:   payloadString(payload, "transcript_path"),
		CWD:          payloadString(payload, "cwd"),
		Model:        payloadString(payload, "model"),
		Source:       payloadString(payload, "source"),
		Permission:   payloadString(payload, "permission_mode"),
		Reason:       payloadString(payload, "reason"),
		Error:        payloadString(payload, "error"),
		ErrorDetails: payloadString(payload, "error_details"),
		Trigger:      payloadString(payload, "trigger"),
	}
	return event, nil
}

func (s *Service) payloadTimestamp(payload map[string]any) time.Time {
	if ts, ok := parseTimestamp(payload["timestamp"]); ok {
		return ts
	}
	return s.now()
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

func parseTimestamp(value any) (time.Time, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return time.Time{}, false
		}
		if ts, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return ts.UTC(), true
		}
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return timestampFromInt(n), true
		}
		return time.Time{}, false
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return timestampFromInt(n), true
		}
		if f, err := v.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			sec, frac := math.Modf(f)
			return time.Unix(int64(sec), int64(frac*1_000_000_000)).UTC(), true
		}
		return time.Time{}, false
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return time.Time{}, false
		}
		sec, frac := math.Modf(v)
		return time.Unix(int64(sec), int64(frac*1_000_000_000)).UTC(), true
	default:
		return time.Time{}, false
	}
}

func timestampFromInt(n int64) time.Time {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1_000_000_000_000_000_000:
		return time.Unix(0, n).UTC()
	case abs >= 1_000_000_000_000:
		return time.UnixMilli(n).UTC()
	default:
		return time.Unix(n, 0).UTC()
	}
}
