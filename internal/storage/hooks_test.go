package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestInsertHookEventAndQueryRecentFailures(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	events := []contracts.HookEvent{
		{Session: "s1", Event: "SessionStart", Timestamp: base, Transcript: "/tmp/s1.jsonl", CWD: "/repo", Model: "claude-opus-4-7", Source: "hook", Permission: "acceptEdits"},
		{Session: "s1", Event: "StopFailure", Timestamp: base.Add(time.Minute), Error: "429", ErrorDetails: "rate limited", Trigger: "stop"},
		{Session: "s2", Event: "StopFailure", Timestamp: base.Add(2 * time.Minute), Error: "auth", ErrorDetails: "expired token", Trigger: "stop"},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Event, err)
		}
	}

	got, err := s.QueryRecentFailures(ctx, "work", base.Add(30*time.Second))
	if err != nil {
		t.Fatalf("QueryRecentFailures: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("failures: got %d want 2", len(got))
	}
	if got[0].Session != "s2" || got[0].Error != "auth" || got[0].ErrorDetails != "expired token" {
		t.Errorf("newest failure = %+v, want s2 auth expired token", got[0])
	}
	if got[1].Session != "s1" || got[1].Error != "429" {
		t.Errorf("older failure = %+v, want s1 429", got[1])
	}
}

func TestQueryHookEventsForSessionFiltersBySessionAndSince(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	base := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	events := []contracts.HookEvent{
		{Session: "sid-1", Event: "SessionStart", Timestamp: base.Add(-time.Minute), Transcript: "/tmp/old.jsonl"},
		{Session: "sid-2", Event: "Stop", Timestamp: base.Add(time.Second), Transcript: "/tmp/other.jsonl"},
		{Session: "sid-1", Event: "PreCompact", Timestamp: base.Add(2 * time.Second), Transcript: "/tmp/sid-1.jsonl"},
		{Session: "sid-1", Event: "Stop", Timestamp: base.Add(3 * time.Second), Transcript: "/tmp/sid-1.jsonl", Reason: "turn-complete"},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s/%s): %v", ev.Session, ev.Event, err)
		}
	}

	got, err := s.QueryHookEventsForSession(ctx, "sid-1", base)
	if err != nil {
		t.Fatalf("QueryHookEventsForSession: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2: %+v", len(got), got)
	}
	if got[0].Event != "PreCompact" || got[1].Event != "Stop" {
		t.Fatalf("events = %v, want PreCompact then Stop", []string{got[0].Event, got[1].Event})
	}
	if got[1].Profile != "work" || got[1].Session != "sid-1" || got[1].Reason != "turn-complete" {
		t.Fatalf("Stop event = %+v, want work/sid-1 with reason", got[1])
	}
}

func TestQueryHookEventsForSessionAfterIDUsesInsertionOrder(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	oldPayloadTime := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	if err := s.InsertHookEvent(ctx, "work", contracts.HookEvent{
		Session:   "sid-1",
		Event:     "SessionStart",
		Timestamp: oldPayloadTime,
	}); err != nil {
		t.Fatalf("InsertHookEvent start: %v", err)
	}
	afterID, err := s.LatestHookEventID(ctx, "sid-1")
	if err != nil {
		t.Fatalf("LatestHookEventID: %v", err)
	}
	if afterID == 0 {
		t.Fatal("LatestHookEventID = 0 after inserted start event")
	}
	if err := s.InsertHookEvent(ctx, "work", contracts.HookEvent{
		Session:   "sid-1",
		Event:     "Stop",
		Timestamp: oldPayloadTime.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("InsertHookEvent stop: %v", err)
	}

	got, err := s.QueryHookEventsForSessionAfterID(ctx, "sid-1", afterID)
	if err != nil {
		t.Fatalf("QueryHookEventsForSessionAfterID: %v", err)
	}
	if len(got) != 1 || got[0].Event != "Stop" {
		t.Fatalf("events after id = %+v, want delayed Stop", got)
	}
}

func TestUpsertSessionTelemetryLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	steps := []contracts.HookEvent{
		{
			Session:    "s1",
			Event:      "SessionStart",
			Timestamp:  start,
			Transcript: "/tmp/s1.jsonl",
			CWD:        "/repo",
			Model:      "claude-opus-4-7",
			Source:     "hook",
			Permission: "acceptEdits",
		},
		{
			Session:      "s1",
			Event:        "StopFailure",
			Timestamp:    start.Add(time.Minute),
			Error:        "429",
			ErrorDetails: "rate limited",
		},
		{
			Session:   "s1",
			Event:     "SessionEnd",
			Timestamp: start.Add(2 * time.Minute),
			Reason:    "stop-hook",
		},
		{
			Session:   "s1",
			Event:     "PostCompact",
			Timestamp: start.Add(3 * time.Minute),
		},
	}
	for _, ev := range steps {
		if err := s.UpsertSessionTelemetry(ctx, "work", ev); err != nil {
			t.Fatalf("UpsertSessionTelemetry(%s): %v", ev.Event, err)
		}
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{
		Profile: "work",
		Status:  "failed",
		Since:   start,
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	session := got[0]
	if session.Profile != "work" || session.Session != "s1" {
		t.Errorf("identity = (%q, %q), want (work, s1)", session.Profile, session.Session)
	}
	if session.Status != "failed" {
		t.Errorf("Status = %q, want failed", session.Status)
	}
	if !session.StartedAt.Equal(start) {
		t.Errorf("StartedAt = %v, want %v", session.StartedAt, start)
	}
	if !session.EndedAt.Equal(start.Add(2 * time.Minute)) {
		t.Errorf("EndedAt = %v, want %v", session.EndedAt, start.Add(2*time.Minute))
	}
	if !session.LastSeenAt.Equal(start.Add(3 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", session.LastSeenAt, start.Add(3*time.Minute))
	}
	if session.FailureError != "429" || session.FailureDetails != "rate limited" {
		t.Errorf("failure fields = (%q, %q), want (429, rate limited)", session.FailureError, session.FailureDetails)
	}
	if session.EndReason != "stop-hook" {
		t.Errorf("EndReason = %q, want stop-hook", session.EndReason)
	}
	if session.CompactCount != 1 {
		t.Errorf("CompactCount = %d, want 1", session.CompactCount)
	}
	if session.Transcript != "/tmp/s1.jsonl" || session.CWD != "/repo" ||
		session.Model != "claude-opus-4-7" || session.Source != "hook" ||
		session.Permission != "acceptEdits" {
		t.Errorf("session metadata not preserved: %+v", session)
	}
}

func TestProfileForSessionReturnsOwner(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionStart",
		Timestamp: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertSessionTelemetry: %v", err)
	}

	profile, ok, err := s.ProfileForSession(ctx, "s1")
	if err != nil {
		t.Fatalf("ProfileForSession: %v", err)
	}
	if !ok || profile != "work" {
		t.Fatalf("ProfileForSession(s1) = (%q, %v), want (work, true)", profile, ok)
	}

	profile, ok, err = s.ProfileForSession(ctx, "missing")
	if err != nil {
		t.Fatalf("ProfileForSession missing: %v", err)
	}
	if ok || profile != "" {
		t.Fatalf("ProfileForSession(missing) = (%q, %v), want empty false", profile, ok)
	}
}

func TestRecordHookTelemetryWritesEventAndSessionAtomically(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 20, 10, 15, 0, 0, time.UTC)
	event := contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    ts,
		Transcript:   "/tmp/s1.jsonl",
		CWD:          "/repo",
		Model:        "claude-sonnet-4-6",
		Error:        "rate_limit",
		ErrorDetails: "429 Too Many Requests",
	}
	if err := s.RecordHookTelemetry(ctx, "work", event); err != nil {
		t.Fatalf("RecordHookTelemetry: %v", err)
	}

	failures, err := s.QueryRecentFailures(ctx, "work", ts.Add(-time.Second))
	if err != nil {
		t.Fatalf("QueryRecentFailures: %v", err)
	}
	if len(failures) != 1 || failures[0].Session != "s1" || failures[0].Error != "rate_limit" {
		t.Fatalf("failures = %+v, want recorded StopFailure", failures)
	}
	sessions, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Status != "failed" || sessions[0].FailureError != "rate_limit" ||
		sessions[0].Transcript != "/tmp/s1.jsonl" || sessions[0].CWD != "/repo" {
		t.Fatalf("session aggregate = %+v, want failed session with event metadata", sessions[0])
	}
}

func TestHookTelemetryRejectsMissingIdentityFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	cases := []struct {
		name    string
		profile string
		event   contracts.HookEvent
	}{
		{
			name:    "missing profile",
			profile: "",
			event: contracts.HookEvent{
				Session: "s1",
				Event:   "SessionStart",
			},
		},
		{
			name:    "missing session",
			profile: "work",
			event: contracts.HookEvent{
				Event: "SessionStart",
			},
		},
		{
			name:    "missing event",
			profile: "work",
			event: contracts.HookEvent{
				Session: "s1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/insert", func(t *testing.T) {
			if err := s.InsertHookEvent(ctx, tc.profile, tc.event); err == nil {
				t.Fatal("InsertHookEvent succeeded, want validation error")
			}
		})
		t.Run(tc.name+"/upsert", func(t *testing.T) {
			if err := s.UpsertSessionTelemetry(ctx, tc.profile, tc.event); err == nil {
				t.Fatal("UpsertSessionTelemetry succeeded, want validation error")
			}
		})
		t.Run(tc.name+"/record", func(t *testing.T) {
			if err := s.RecordHookTelemetry(ctx, tc.profile, tc.event); err == nil {
				t.Fatal("RecordHookTelemetry succeeded, want validation error")
			}
		})
	}
}

func TestQuerySessionsReturnsEmptySliceWhenNoRows(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if got == nil {
		t.Fatal("QuerySessions returned nil slice, want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("sessions length = %d, want 0", len(got))
	}
}

func TestUpsertSessionTelemetryStopCompletesSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionStart",
		Timestamp: start,
	}); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:    "s1",
		Event:      "Stop",
		Timestamp:  start.Add(time.Minute),
		Transcript: "/tmp/s1-updated.jsonl",
		CWD:        "/repo",
		Permission: "default",
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work", Status: "completed"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "completed" || got[0].FailureError != "" || got[0].FailureDetails != "" {
		t.Errorf("completed session has unexpected state: %+v", got[0])
	}
	if got[0].Transcript != "/tmp/s1-updated.jsonl" || got[0].CWD != "/repo" || got[0].Permission != "default" {
		t.Errorf("Stop metadata not stored: %+v", got[0])
	}
}

func TestUpsertSessionTelemetryOutOfOrderStartPreservesFailedStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "429",
		ErrorDetails: "rate limited",
	}); err != nil {
		t.Fatalf("StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionStart",
		Timestamp: start,
		Model:     "claude-opus-4-7",
	}); err != nil {
		t.Fatalf("late SessionStart: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", got[0].Status)
	}
	if !got[0].LastSeenAt.Equal(start.Add(time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(time.Minute))
	}
	if got[0].FailureError != "429" || got[0].FailureDetails != "rate limited" {
		t.Errorf("failure fields = (%q, %q), want (429, rate limited)", got[0].FailureError, got[0].FailureDetails)
	}
	if got[0].Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want claude-opus-4-7 from late SessionStart", got[0].Model)
	}
}

func TestUpsertSessionTelemetryLateStopFailurePreservesEndFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionEnd",
		Timestamp: start.Add(2 * time.Minute),
		Reason:    "turn-complete",
	}); err != nil {
		t.Fatalf("SessionEnd: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "429",
		ErrorDetails: "rate limited",
	}); err != nil {
		t.Fatalf("late StopFailure: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", got[0].Status)
	}
	if got[0].FailureError != "429" || got[0].FailureDetails != "rate limited" {
		t.Errorf("failure fields = (%q, %q), want (429, rate limited)", got[0].FailureError, got[0].FailureDetails)
	}
	if !got[0].EndedAt.Equal(start.Add(2 * time.Minute)) {
		t.Errorf("EndedAt = %v, want %v", got[0].EndedAt, start.Add(2*time.Minute))
	}
	if got[0].EndReason != "turn-complete" {
		t.Errorf("EndReason = %q, want turn-complete", got[0].EndReason)
	}
	if !got[0].LastSeenAt.Equal(start.Add(2 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(2*time.Minute))
	}
}

func TestUpsertSessionTelemetryLateSessionEndPreservesFailedStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 12, 40, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(2 * time.Minute),
		Error:        "429",
		ErrorDetails: "rate limited",
	}); err != nil {
		t.Fatalf("StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionEnd",
		Timestamp: start.Add(time.Minute),
		Reason:    "stop-hook",
	}); err != nil {
		t.Fatalf("late SessionEnd: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", got[0].Status)
	}
	if !got[0].EndedAt.Equal(start.Add(time.Minute)) {
		t.Errorf("EndedAt = %v, want %v", got[0].EndedAt, start.Add(time.Minute))
	}
	if got[0].EndReason != "stop-hook" {
		t.Errorf("EndReason = %q, want stop-hook", got[0].EndReason)
	}
	if !got[0].LastSeenAt.Equal(start.Add(2 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(2*time.Minute))
	}
}

func TestUpsertSessionTelemetryOlderStopDoesNotDowngradeEndedStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 12, 42, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionEnd",
		Timestamp: start.Add(2 * time.Minute),
		Reason:    "turn-complete",
	}); err != nil {
		t.Fatalf("SessionEnd: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: start.Add(time.Minute),
	}); err != nil {
		t.Fatalf("late Stop: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "ended" {
		t.Errorf("Status = %q, want ended", got[0].Status)
	}
	if !got[0].LastSeenAt.Equal(start.Add(2 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(2*time.Minute))
	}
	if got[0].EndReason != "turn-complete" {
		t.Errorf("EndReason = %q, want turn-complete", got[0].EndReason)
	}
}

func TestUpsertSessionTelemetryLateSessionStartPreservesMetadataWithoutStatusRegression(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 12, 45, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: start.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:    "s1",
		Event:      "SessionStart",
		Timestamp:  start,
		Transcript: "/tmp/s1.jsonl",
		CWD:        "/repo",
		Model:      "claude-sonnet-4-6",
		Source:     "hook",
		Permission: "acceptEdits",
	}); err != nil {
		t.Fatalf("late SessionStart: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "completed" {
		t.Errorf("Status = %q, want completed", got[0].Status)
	}
	if !got[0].StartedAt.Equal(start) {
		t.Errorf("StartedAt = %v, want %v", got[0].StartedAt, start)
	}
	if got[0].Transcript != "/tmp/s1.jsonl" || got[0].CWD != "/repo" ||
		got[0].Model != "claude-sonnet-4-6" || got[0].Source != "hook" ||
		got[0].Permission != "acceptEdits" {
		t.Errorf("late SessionStart metadata not merged: %+v", got[0])
	}
	if !got[0].LastSeenAt.Equal(start.Add(time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(time.Minute))
	}
}

func TestUpsertSessionTelemetryNewerStopFailureKeepsNewestFailureFacts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 13, 15, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(2 * time.Minute),
		Error:        "newer-429",
		ErrorDetails: "newer rate limit",
	}); err != nil {
		t.Fatalf("newer StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "older-auth",
		ErrorDetails: "older token expired",
	}); err != nil {
		t.Fatalf("older StopFailure: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", got[0].Status)
	}
	if got[0].FailureError != "newer-429" || got[0].FailureDetails != "newer rate limit" {
		t.Errorf("failure fields = (%q, %q), want (newer-429, newer rate limit)", got[0].FailureError, got[0].FailureDetails)
	}
}

func TestUpsertSessionTelemetryLaterStopFailureUpdatesFailureFacts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "older-auth",
		ErrorDetails: "older token expired",
	}); err != nil {
		t.Fatalf("older StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(2 * time.Minute),
		Error:        "newer-429",
		ErrorDetails: "newer rate limit",
	}); err != nil {
		t.Fatalf("newer StopFailure: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", got[0].Status)
	}
	if got[0].FailureError != "newer-429" || got[0].FailureDetails != "newer rate limit" {
		t.Errorf("failure fields = (%q, %q), want (newer-429, newer rate limit)", got[0].FailureError, got[0].FailureDetails)
	}
}

func TestUpsertSessionTelemetryNewerStopFailureWithBlankDetailsClearsOlderDetails(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 13, 45, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "older-auth",
		ErrorDetails: "older token expired",
	}); err != nil {
		t.Fatalf("older StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "StopFailure",
		Timestamp: start.Add(2 * time.Minute),
		Error:     "newer-429",
	}); err != nil {
		t.Fatalf("newer StopFailure: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].FailureError != "newer-429" || got[0].FailureDetails != "" {
		t.Errorf("failure fields = (%q, %q), want (newer-429, empty)", got[0].FailureError, got[0].FailureDetails)
	}
}

func TestUpsertSessionTelemetryNewerStopFailureWithBlankErrorClearsOlderError(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 14, 0, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "older-auth",
		ErrorDetails: "older token expired",
	}); err != nil {
		t.Fatalf("older StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(2 * time.Minute),
		ErrorDetails: "newer detail only",
	}); err != nil {
		t.Fatalf("newer StopFailure: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].FailureError != "" || got[0].FailureDetails != "newer detail only" {
		t.Errorf("failure fields = (%q, %q), want (empty, newer detail only)", got[0].FailureError, got[0].FailureDetails)
	}
}

func TestUpsertSessionTelemetryNewerBlankStopFailureClearsOlderFailureFacts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 14, 15, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    start.Add(time.Minute),
		Error:        "older-auth",
		ErrorDetails: "older token expired",
	}); err != nil {
		t.Fatalf("older StopFailure: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "StopFailure",
		Timestamp: start.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("newer blank StopFailure: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", got[0].Status)
	}
	if got[0].FailureError != "" || got[0].FailureDetails != "" {
		t.Errorf("failure fields = (%q, %q), want both empty", got[0].FailureError, got[0].FailureDetails)
	}
}

func TestUpsertSessionTelemetrySameTimestampStopFailureUsesDeterministicTieBreak(t *testing.T) {
	ctx := context.Background()
	ts := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	lower := contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    ts,
		Error:        "alpha",
		ErrorDetails: "zulu",
	}
	higher := contracts.HookEvent{
		Session:      "s1",
		Event:        "StopFailure",
		Timestamp:    ts,
		Error:        "bravo",
		ErrorDetails: "",
	}

	first := newTestStore(t)
	mustSaveProfile(t, first, "work")
	if err := first.UpsertSessionTelemetry(ctx, "work", lower); err != nil {
		t.Fatalf("first lower StopFailure: %v", err)
	}
	if err := first.UpsertSessionTelemetry(ctx, "work", higher); err != nil {
		t.Fatalf("first higher StopFailure: %v", err)
	}

	second := newTestStore(t)
	mustSaveProfile(t, second, "work")
	if err := second.UpsertSessionTelemetry(ctx, "work", higher); err != nil {
		t.Fatalf("second higher StopFailure: %v", err)
	}
	if err := second.UpsertSessionTelemetry(ctx, "work", lower); err != nil {
		t.Fatalf("second lower StopFailure: %v", err)
	}

	firstRows, err := first.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("first QuerySessions: %v", err)
	}
	secondRows, err := second.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("second QuerySessions: %v", err)
	}
	if len(firstRows) != 1 || len(secondRows) != 1 {
		t.Fatalf("session lens = (%d, %d), want (1, 1)", len(firstRows), len(secondRows))
	}
	if firstRows[0].FailureError != "bravo" || firstRows[0].FailureDetails != "" {
		t.Errorf("first failure fields = (%q, %q), want (bravo, empty)", firstRows[0].FailureError, firstRows[0].FailureDetails)
	}
	if secondRows[0].FailureError != "bravo" || secondRows[0].FailureDetails != "" {
		t.Errorf("second failure fields = (%q, %q), want (bravo, empty)", secondRows[0].FailureError, secondRows[0].FailureDetails)
	}
}

func TestUpsertSessionTelemetryUnknownEventPreservesCompletedStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionStart",
		Timestamp: start,
	}); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: start.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "FutureHook",
		Timestamp: start.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("FutureHook: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "completed" {
		t.Errorf("Status = %q, want completed", got[0].Status)
	}
	if !got[0].LastSeenAt.Equal(start.Add(2 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(2*time.Minute))
	}
}

func TestUpsertSessionTelemetryOlderEventDoesNotMoveLastSeenBackward(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 14, 0, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionStart",
		Timestamp: start,
	}); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: start.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "PostCompact",
		Timestamp: start.Add(time.Minute),
	}); err != nil {
		t.Fatalf("old PostCompact: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "completed" {
		t.Errorf("Status = %q, want completed", got[0].Status)
	}
	if !got[0].LastSeenAt.Equal(start.Add(5 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(5*time.Minute))
	}
	if got[0].CompactCount != 1 {
		t.Errorf("CompactCount = %d, want 1 for observed older compact event", got[0].CompactCount)
	}
}

func TestUpsertSessionTelemetryOlderUnknownEventDoesNotChangeStatusOrLastSeen(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	start := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: start.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "FutureHook",
		Timestamp: start.Add(time.Minute),
	}); err != nil {
		t.Fatalf("old FutureHook: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].Status != "completed" {
		t.Errorf("Status = %q, want completed", got[0].Status)
	}
	if !got[0].LastSeenAt.Equal(start.Add(5 * time.Minute)) {
		t.Errorf("LastSeenAt = %v, want %v", got[0].LastSeenAt, start.Add(5*time.Minute))
	}
}

func TestUpsertSessionTelemetryDuplicatePostCompactCountsObservedEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 20, 15, 0, 0, 0, time.UTC)
	event := contracts.HookEvent{
		Session:   "s1",
		Event:     "PostCompact",
		Timestamp: ts,
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", event); err != nil {
		t.Fatalf("first PostCompact: %v", err)
	}
	if err := s.UpsertSessionTelemetry(ctx, "work", event); err != nil {
		t.Fatalf("duplicate PostCompact: %v", err)
	}

	got, err := s.QuerySessions(ctx, contracts.SessionQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions: got %d want 1", len(got))
	}
	if got[0].CompactCount != 2 {
		t.Errorf("CompactCount = %d, want 2 because session aggregates have no hook event id dedupe", got[0].CompactCount)
	}
}

func TestProfileHealthSaveGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	checked := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	in := contracts.ProfileHealth{
		Profile:    "work",
		CheckedAt:  checked,
		AuthStatus: "ok",
		AuthDetail: "token valid",
	}
	if err := s.SaveProfileHealth(ctx, in); err != nil {
		t.Fatalf("SaveProfileHealth: %v", err)
	}

	got, err := s.GetProfileHealth(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfileHealth: %v", err)
	}
	if got.Profile != in.Profile || got.AuthStatus != in.AuthStatus ||
		got.AuthDetail != in.AuthDetail || !got.CheckedAt.Equal(in.CheckedAt) {
		t.Errorf("profile health mismatch:\n got  %+v\n want %+v", got, in)
	}
}

func TestGetProfileHealthMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.GetProfileHealth(ctx, "ghost")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}
