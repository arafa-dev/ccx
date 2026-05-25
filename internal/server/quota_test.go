package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/server"
)

type stubQuotaProvider struct {
	rows []contracts.ProfileQuota
	err  error
}

func (s *stubQuotaProvider) Quota(_ context.Context, profileFilter string) ([]contracts.ProfileQuota, error) {
	if s.err != nil {
		return nil, s.err
	}
	if profileFilter == "" {
		return s.rows, nil
	}
	out := []contracts.ProfileQuota{}
	for _, r := range s.rows {
		if r.Profile == profileFilter {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestHandleQuotaAllProfiles(t *testing.T) {
	rows := []contracts.ProfileQuota{
		{
			Profile:  "work",
			PlanTier: "max20",
			Window5h: contracts.QuotaWindow{
				Used:     142,
				Cap:      900,
				Pct:      15.78,
				ResetsAt: time.Now().Add(time.Hour),
			},
		},
		{Profile: "personal", PlanTier: "pro", Window5h: contracts.QuotaWindow{Used: 45, Cap: 45, Pct: 100}},
	}
	srv := server.New(server.Deps{Quota: &stubQuotaProvider{rows: rows}}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []contracts.ProfileQuota
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("rows: got %d, want 2", len(got))
	}
}

func TestHandleQuotaProfileFilter(t *testing.T) {
	rows := []contracts.ProfileQuota{
		{Profile: "work"},
		{Profile: "personal"},
	}
	srv := server.New(server.Deps{Quota: &stubQuotaProvider{rows: rows}}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota?profile=work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var got []contracts.ProfileQuota
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Profile != "work" {
		t.Errorf("got %+v, want single 'work' row", got)
	}
}

func TestHandleQuotaProviderMissingReturns503(t *testing.T) {
	srv := server.New(server.Deps{}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleQuotaProviderErrorReturns500(t *testing.T) {
	srv := server.New(server.Deps{Quota: &stubQuotaProvider{err: errors.New("quota failed")}}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
