package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/recstream"
	"github.com/arafa-dev/ccx/internal/server"
)

func TestRecommendationsLiveEmitsPublishedEvent(t *testing.T) {
	hub := recstream.NewHub()
	defer hub.Close()
	srv := server.New(server.Deps{Recommendations: hub}, "test")

	rec := newFlushingRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/recommendations/live", nil).WithContext(ctx)

	served := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rec, req)
		close(served)
	}()

	select {
	case <-rec.flushed:
	case <-time.After(time.Second):
		t.Fatal("initial flush missing")
	}

	hub.Publish(contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationWarn})
	select {
	case <-rec.flushed:
	case <-time.After(time.Second):
		t.Fatal("handler did not flush the published event within 1s")
	}

	cancel()
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after ctx cancel within 1s")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: recommendation") {
		t.Errorf("body missing recommendation event: %s", body)
	}
	if !strings.Contains(body, `"profile":"work"`) {
		t.Errorf("body missing profile JSON: %s", body)
	}
}

type flushingRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func newFlushingRecorder() *flushingRecorder {
	return &flushingRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		flushed:          make(chan struct{}, 8),
	}
}

func (f *flushingRecorder) Flush() {
	f.ResponseRecorder.Flush()
	select {
	case f.flushed <- struct{}{}:
	default:
	}
}
