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
