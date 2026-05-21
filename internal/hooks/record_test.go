package hooks

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestRecordParsesHookPayloadsAndStoresTelemetry(t *testing.T) {
	ctx := context.Background()
	profile := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}
	store := &fakeHookStore{}
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	svc := testService(profile)
	svc.Store = store
	svc.Now = func() time.Time { return now }

	cases := []struct {
		name string
		in   string
		want contracts.HookEvent
	}{
		{
			name: "SessionStart",
			in: `{
  "session_id": "sess-start",
  "transcript_path": "/tmp/start.jsonl",
  "cwd": "/repo",
  "hook_event_name": "SessionStart",
  "source": "startup",
  "model": "claude-sonnet-4-6",
  "permission_mode": "acceptEdits",
  "timestamp": "2026-05-22T10:00:00Z"
}`,
			want: contracts.HookEvent{
				Profile:    "work",
				Session:    "sess-start",
				Event:      "SessionStart",
				Timestamp:  time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
				Transcript: "/tmp/start.jsonl",
				CWD:        "/repo",
				Model:      "claude-sonnet-4-6",
				Source:     "startup",
				Permission: "acceptEdits",
			},
		},
		{
			name: "StopFailure",
			in: `{
  "session_id": "sess-fail",
  "transcript_path": "/tmp/fail.jsonl",
  "cwd": "/repo",
  "hook_event_name": "StopFailure",
  "error": "rate_limit",
  "error_details": "429 Too Many Requests",
  "trigger": "stop"
}`,
			want: contracts.HookEvent{
				Profile:      "work",
				Session:      "sess-fail",
				Event:        "StopFailure",
				Timestamp:    now,
				Transcript:   "/tmp/fail.jsonl",
				CWD:          "/repo",
				Error:        "rate_limit",
				ErrorDetails: "429 Too Many Requests",
				Trigger:      "stop",
			},
		},
		{
			name: "SessionEnd",
			in: `{
  "session_id": "sess-end",
  "transcript_path": "/tmp/end.jsonl",
  "cwd": "/repo",
  "hook_event_name": "SessionEnd",
  "reason": "prompt_input_exit",
  "timestamp": 1770000000000000000
}`,
			want: contracts.HookEvent{
				Profile:    "work",
				Session:    "sess-end",
				Event:      "SessionEnd",
				Timestamp:  time.Unix(0, 1770000000000000000).UTC(),
				Transcript: "/tmp/end.jsonl",
				CWD:        "/repo",
				Reason:     "prompt_input_exit",
			},
		},
		{
			name: "PostCompact",
			in: `{
  "session_id": "sess-compact",
  "transcript_path": "/tmp/compact.jsonl",
  "cwd": "/repo",
  "hook_event_name": "PostCompact",
  "trigger": "manual",
  "timestamp": "2026-05-22T11:30:00+01:00"
}`,
			want: contracts.HookEvent{
				Profile:    "work",
				Session:    "sess-compact",
				Event:      "PostCompact",
				Timestamp:  time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC),
				Transcript: "/tmp/compact.jsonl",
				CWD:        "/repo",
				Trigger:    "manual",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeInsert := len(store.inserted)
			beforeUpsert := len(store.upserted)
			result, err := svc.Record(ctx, RecordOptions{
				Profile: "work",
				Input:   strings.NewReader(tc.in),
			})
			if err != nil {
				t.Fatalf("Record: %v", err)
			}
			if !result.Recorded || result.Profile != "work" || result.Event != tc.want.Event {
				t.Fatalf("result = %+v, want recorded event %s", result, tc.want.Event)
			}
			if len(store.inserted) != beforeInsert+1 {
				t.Fatalf("insert calls = %d, want %d", len(store.inserted), beforeInsert+1)
			}
			if len(store.upserted) != beforeUpsert+1 {
				t.Fatalf("upsert calls = %d, want %d", len(store.upserted), beforeUpsert+1)
			}
			if got := store.inserted[len(store.inserted)-1]; got.profile != "work" || got.event != tc.want {
				t.Fatalf("InsertHookEvent = (%q, %+v), want (work, %+v)", got.profile, got.event, tc.want)
			}
			if got := store.upserted[len(store.upserted)-1]; got.profile != "work" || got.event != tc.want {
				t.Fatalf("UpsertSessionTelemetry = (%q, %+v), want (work, %+v)", got.profile, got.event, tc.want)
			}
		})
	}
}

func TestRecordRequiresRegisteredProfileAndProfileFlag(t *testing.T) {
	ctx := context.Background()
	profile := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}
	svc := testService(profile)
	svc.Store = &fakeHookStore{}

	_, err := svc.Record(ctx, RecordOptions{
		Input: strings.NewReader(`{"session_id":"s1","hook_event_name":"SessionStart"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("missing profile error = %v, want profile error", err)
	}

	_, err = svc.Record(ctx, RecordOptions{
		Profile: "missing",
		Input:   strings.NewReader(`{"session_id":"s1","hook_event_name":"SessionStart"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unregistered profile error = %v, want missing profile error", err)
	}
}

func TestRecordRejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	profile := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}
	store := &fakeHookStore{}
	svc := testService(profile)
	svc.Store = store

	_, err := svc.Record(ctx, RecordOptions{
		Profile: "work",
		Input:   strings.NewReader(`{"session_id":`),
	})
	if err == nil {
		t.Fatalf("Record succeeded, want invalid JSON error")
	}
	if len(store.inserted) != 0 || len(store.upserted) != 0 {
		t.Fatalf("store calls after invalid JSON: inserts=%d upserts=%d", len(store.inserted), len(store.upserted))
	}
}

func TestRecordRejectsMalformedPayloadsBeforeStoreWrites(t *testing.T) {
	ctx := context.Background()
	profile := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "missing session id",
			in:   `{"hook_event_name":"SessionStart"}`,
			want: "session_id",
		},
		{
			name: "trailing JSON",
			in:   `{"session_id":"s1","hook_event_name":"SessionStart"} {"session_id":"s2"}`,
			want: "trailing",
		},
		{
			name: "oversized input",
			in:   `{"session_id":"s1","hook_event_name":"SessionStart","padding":"` + strings.Repeat("x", 1024*1024) + `"}`,
			want: "too large",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeHookStore{}
			svc := testService(profile)
			svc.Store = store

			_, err := svc.Record(ctx, RecordOptions{
				Profile: "work",
				Input:   strings.NewReader(tc.in),
			})
			if err == nil {
				t.Fatalf("Record succeeded, want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
			if len(store.saved) != 0 || len(store.inserted) != 0 || len(store.upserted) != 0 {
				t.Fatalf("store writes after invalid payload: saves=%d inserts=%d upserts=%d", len(store.saved), len(store.inserted), len(store.upserted))
			}
		})
	}
}

type fakeHookStore struct {
	saved    []contracts.Profile
	inserted []storedHookEvent
	upserted []storedHookEvent
}

type storedHookEvent struct {
	profile string
	event   contracts.HookEvent
}

func (f *fakeHookStore) SaveProfile(ctx context.Context, profile contracts.Profile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.saved = append(f.saved, profile)
	return nil
}

func (f *fakeHookStore) InsertHookEvent(ctx context.Context, profileName string, event contracts.HookEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.inserted = append(f.inserted, storedHookEvent{profile: profileName, event: event})
	return nil
}

func (f *fakeHookStore) UpsertSessionTelemetry(ctx context.Context, profileName string, event contracts.HookEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.upserted = append(f.upserted, storedHookEvent{profile: profileName, event: event})
	return nil
}
