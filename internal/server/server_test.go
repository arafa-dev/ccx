package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/server"
)

func TestHealthEndpoint(t *testing.T) {
	srv := server.New(server.Deps{Store: &mockStore{}, Pricing: &mockPricing{}}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Errorf("status: %d", res.StatusCode)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	if !body.OK || body.Version != "test" {
		t.Errorf("got %+v", body)
	}
}

func TestProfilesEndpointReportsUsageQueryErrors(t *testing.T) {
	srv := server.New(server.Deps{
		Store:   &mockStore{queryErr: errors.New("store unavailable")},
		Pricing: &mockPricing{},
		Profiles: mockProfiles{profiles: []contracts.Profile{{
			Name:      "demo",
			ConfigDir: "/tmp/demo",
		}}},
	}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(body.Error, "query usage for profile \"demo\"") ||
		!strings.Contains(body.Error, "store unavailable") {
		t.Fatalf("error = %q", body.Error)
	}
}

func TestProfilesEndpointReportsPricingErrors(t *testing.T) {
	srv := server.New(server.Deps{
		Store: &mockStore{queryRows: []contracts.UsageRow{{
			Model: "model-a",
			Day:   time.Now().UTC(),
			Usage: contracts.Usage{InputTokens: 1},
		}}},
		Pricing: &mockPricing{err: errors.New("pricing unavailable")},
		Profiles: mockProfiles{profiles: []contracts.Profile{{
			Name:      "demo",
			ConfigDir: "/tmp/demo",
		}}},
	}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(body.Error, "pricing for profile \"demo\"") ||
		!strings.Contains(body.Error, "pricing unavailable") {
		t.Fatalf("error = %q", body.Error)
	}
}

func TestUsageEndpointReportsPricingErrors(t *testing.T) {
	srv := server.New(server.Deps{
		Store: &mockStore{queryRows: []contracts.UsageRow{{
			Model: "model-a",
			Day:   time.Now().UTC(),
			Usage: contracts.Usage{InputTokens: 1},
		}}},
		Pricing: &mockPricing{err: errors.New("pricing unavailable")},
	}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/usage")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(body.Error, "pricing error") ||
		!strings.Contains(body.Error, "pricing unavailable") {
		t.Fatalf("error = %q", body.Error)
	}
}

func TestUsageEndpointRejectsNonPositiveSince(t *testing.T) {
	srv := server.New(server.Deps{Store: &mockStore{}, Pricing: &mockPricing{}}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, since := range []string{"0", "0d", "-1h"} {
		res, err := ts.Client().Get(ts.URL + "/api/usage?since=" + since)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("since=%s status = %d, want %d", since, res.StatusCode, http.StatusBadRequest)
		}
	}
}

type mockStore struct {
	contracts.Store
	queryRows []contracts.UsageRow
	queryErr  error
}

func (m *mockStore) ListProfiles(_ context.Context) ([]contracts.Profile, error) {
	return []contracts.Profile{{Name: "demo", ConfigDir: "/tmp/demo"}}, nil
}

func (m *mockStore) QueryUsage(_ context.Context, _ contracts.UsageQuery) ([]contracts.UsageRow, error) {
	return m.queryRows, m.queryErr
}

type mockPricing struct {
	cost float64
	err  error
}

func (m *mockPricing) Cost(_ string, _ time.Time, _ contracts.Usage) (float64, error) {
	return m.cost, m.err
}

func (m *mockPricing) LastUpdated() time.Time { return time.Time{} }

type mockProfiles struct {
	profiles []contracts.Profile
	err      error
}

func (m mockProfiles) List(_ context.Context) ([]contracts.Profile, error) {
	return m.profiles, m.err
}
