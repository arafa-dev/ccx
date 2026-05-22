package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/hooks"
	"github.com/arafa-dev/ccx/internal/server"
)

func TestDaemonStatusEndpointFallsBackToForegroundWithoutProvider(t *testing.T) {
	srv := server.New(server.Deps{Store: &mockStore{}, Pricing: &mockPricing{}}, "test-version")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/daemon/status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	var body struct {
		Mode    string `json:"mode"`
		Status  string `json:"status"`
		Version string `json:"version"`
		Running bool   `json:"running"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Mode != "foreground" || body.Status != "running" || body.Version != "test-version" || body.Running {
		t.Fatalf("body = %+v, want foreground running dashboard status", body)
	}
}

func TestDaemonStatusEndpointUsesRunningDaemonProvider(t *testing.T) {
	started := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	srv := server.New(server.Deps{
		Store:   &mockStore{},
		Pricing: &mockPricing{},
		Daemon: fakeDaemonStatusProvider{status: contracts.DaemonStatus{
			PID:             1234,
			Version:         "0.1.0-test",
			StartedAt:       started,
			Port:            7777,
			URL:             "http://127.0.0.1:7777",
			DBPath:          "/tmp/ccx/state.db",
			LogPath:         "/tmp/ccx/daemon.log",
			ProfilesWatched: 3,
			Running:         true,
		}},
	}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/daemon/status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	var body struct {
		Mode            string `json:"mode"`
		Status          string `json:"status"`
		PID             int    `json:"pid"`
		Port            int    `json:"port"`
		URL             string `json:"url"`
		ProfilesWatched int    `json:"profiles_watched"`
		Running         bool   `json:"running"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Mode != "daemon" || body.Status != "running" || !body.Running ||
		body.PID != 1234 || body.Port != 7777 || body.URL == "" || body.ProfilesWatched != 3 {
		t.Fatalf("body = %+v, want running daemon status", body)
	}
}

func TestHooksStatusEndpointUsesHooksStatusLogic(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	profiles := []contracts.Profile{
		hookProfile(root, "disabled"),
		hookProfile(root, "installed"),
		hookProfile(root, "invalid"),
		hookProfile(root, "partial"),
	}
	writeSettings(t, profiles[0].ConfigDir, `{"disableAllHooks":true}`)
	writeSettings(t, profiles[2].ConfigDir, `{not-json`)
	writeSettings(t, profiles[3].ConfigDir, `{"hooks":{"Stop":[]}}`)

	source := hookProfileSource{profiles: profiles}
	hookSvc := &hooks.Service{
		Profiles:   source,
		BinaryPath: func() (string, error) { return "/usr/local/bin/ccx-test", nil },
		Now:        func() time.Time { return time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC) },
	}
	if _, err := hookSvc.Install(ctx, hooks.InstallOptions{Profile: "installed"}); err != nil {
		t.Fatalf("install fixture hooks: %v", err)
	}

	srv := server.New(server.Deps{
		Store:   &mockStore{},
		Pricing: &mockPricing{},
		Hooks:   hookSvc,
	}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/hooks/status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	var body []hooks.Result
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := map[string]hooks.Status{}
	for _, result := range body {
		got[result.Profile] = result.Status
		if result.SettingsPath == "" {
			t.Fatalf("settings_path missing for %+v", result)
		}
	}
	want := map[string]hooks.Status{
		"disabled":  hooks.StatusDisabled,
		"installed": hooks.StatusInstalled,
		"invalid":   hooks.StatusInvalid,
		"partial":   hooks.StatusPartial,
	}
	for profile, status := range want {
		if got[profile] != status {
			t.Fatalf("%s status = %q, want %q; all statuses: %+v", profile, got[profile], status, got)
		}
	}
}

func TestSessionsEndpointFiltersAndSortsNewestFirst(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		sessions: []contracts.SessionTelemetry{
			{Profile: "work", Session: "older", Status: "failed", LastSeenAt: now.Add(-90 * time.Minute)},
			{Profile: "work", Session: "newer", Status: "failed", LastSeenAt: now.Add(-10 * time.Minute)},
		},
	}
	srv := server.New(server.Deps{Store: store, Pricing: &mockPricing{}}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/sessions?profile=work&status=failed&since=2h")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if store.lastSessionQuery.Profile != "work" || store.lastSessionQuery.Status != "failed" {
		t.Fatalf("query = %+v, want profile/status filters", store.lastSessionQuery)
	}
	if store.lastSessionQuery.Limit != 50 {
		t.Fatalf("query limit = %d, want 50", store.lastSessionQuery.Limit)
	}
	if since := store.lastSessionQuery.Since; since.Before(now.Add(-3*time.Hour)) || since.After(now.Add(-1*time.Hour)) {
		t.Fatalf("query since = %s, want roughly now-2h", since)
	}
	var body []contracts.SessionTelemetry
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := []string{body[0].Session, body[1].Session}; !slices.Equal(got, []string{"newer", "older"}) {
		t.Fatalf("sessions = %+v, want newest first", got)
	}
}

func TestSessionsEndpointDoesNotApplySinceFilterUnlessProvided(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		sessions: []contracts.SessionTelemetry{
			{Profile: "work", Session: "old", Status: "completed", LastSeenAt: now.Add(-48 * time.Hour)},
			{Profile: "work", Session: "new", Status: "completed", LastSeenAt: now.Add(-2 * time.Hour)},
		},
	}
	srv := server.New(server.Deps{Store: store, Pricing: &mockPricing{}}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if !store.lastSessionQuery.Since.IsZero() {
		t.Fatalf("query since = %s, want zero time when since is omitted", store.lastSessionQuery.Since)
	}
	var unfiltered []contracts.SessionTelemetry
	if err := json.NewDecoder(res.Body).Decode(&unfiltered); err != nil {
		t.Fatalf("decode unfiltered response: %v", err)
	}
	if got := sessionNames(unfiltered); !slices.Equal(got, []string{"new", "old"}) {
		t.Fatalf("unfiltered sessions = %+v, want new and old", got)
	}

	res, err = ts.Client().Get(ts.URL + "/api/sessions?since=24h")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status with since = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if store.lastSessionQuery.Since.IsZero() {
		t.Fatalf("query since is zero, want parsed since when provided")
	}
	var filtered []contracts.SessionTelemetry
	if err := json.NewDecoder(res.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered response: %v", err)
	}
	if got := sessionNames(filtered); !slices.Equal(got, []string{"new"}) {
		t.Fatalf("filtered sessions = %+v, want only new", got)
	}
}

func TestHeadroomEndpointReturnsRecommendationAndUnavailableCandidates(t *testing.T) {
	goodDir := t.TempDir()
	profiles := mockProfiles{profiles: []contracts.Profile{
		{Name: "bad", ConfigDir: filepath.Join(t.TempDir(), "missing")},
		{Name: "good", ConfigDir: goodDir, Limits: contracts.ProfileLimits{DailyTokenBudget: 100_000, Priority: 10}},
	}}
	store := &mockStore{
		usageByProfile: map[string][]contracts.UsageRow{
			"good": {{
				Profile: "good",
				Model:   "claude-sonnet-4-6",
				Day:     time.Now().UTC(),
				Usage:   contracts.Usage{InputTokens: 1000, OutputTokens: 500},
			}},
		},
	}
	srv := server.New(server.Deps{
		Store:    store,
		Pricing:  &mockPricing{},
		Profiles: profiles,
		Headroom: headroom.Evaluator{Store: store, Pricing: &mockPricing{}},
	}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/headroom")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	var body headroom.Result
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Recommendation == nil || body.Recommendation.Profile != "good" {
		t.Fatalf("recommendation = %+v, want good", body.Recommendation)
	}
	badIdx := slices.IndexFunc(body.Candidates, func(c headroom.Candidate) bool { return c.Profile == "bad" })
	if badIdx < 0 || body.Candidates[badIdx].Available {
		t.Fatalf("bad candidate = %+v, want present and unavailable", body.Candidates)
	}
}

func sessionNames(sessions []contracts.SessionTelemetry) []string {
	names := make([]string, 0, len(sessions))
	for _, session := range sessions {
		names = append(names, session.Session)
	}
	return names
}

type fakeDaemonStatusProvider struct {
	status contracts.DaemonStatus
	err    error
}

func (f fakeDaemonStatusProvider) Status(context.Context) (contracts.DaemonStatus, error) {
	return f.status, f.err
}

type hookProfileSource struct {
	profiles []contracts.Profile
}

func (h hookProfileSource) List(context.Context) ([]contracts.Profile, error) {
	return append([]contracts.Profile(nil), h.profiles...), nil
}

func (h hookProfileSource) Get(_ context.Context, name string) (contracts.Profile, error) {
	for _, p := range h.profiles {
		if p.Name == name {
			return p, nil
		}
	}
	return contracts.Profile{}, errors.New("profile not found")
}

func hookProfile(root, name string) contracts.Profile {
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		panic(err)
	}
	return contracts.Profile{Name: name, ConfigDir: dir}
}

func writeSettings(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}
