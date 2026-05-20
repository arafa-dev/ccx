package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

type mockStore struct{ contracts.Store }

func (m *mockStore) ListProfiles(_ context.Context) ([]contracts.Profile, error) {
	return []contracts.Profile{{Name: "demo", ConfigDir: "/tmp/demo"}}, nil
}

func (m *mockStore) QueryUsage(_ context.Context, _ contracts.UsageQuery) ([]contracts.UsageRow, error) {
	return nil, nil
}

type mockPricing struct{}

func (m *mockPricing) Cost(_ string, _ time.Time, _ contracts.Usage) (float64, error) {
	return 0, nil
}

func (m *mockPricing) LastUpdated() time.Time { return time.Time{} }
