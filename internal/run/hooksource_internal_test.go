package run

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestReadRecommendationSSEReturnsWhenContextCanceledWithFullOutput(t *testing.T) {
	event := `{"profile":"work","level":"hard","reason":"cap","timestamp":"2026-05-25T11:00:00Z"}`
	var stream strings.Builder
	for range 4 {
		_, _ = fmt.Fprintf(&stream, "event: recommendation\ndata: %s\n\n", event)
	}

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan contracts.RecommendationEvent, 1)
	done := make(chan struct{})
	go func() {
		readRecommendationSSE(ctx, strings.NewReader(stream.String()), out)
		close(done)
	}()

	deadline := time.Now().Add(100 * time.Millisecond)
	for len(out) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(out) == 0 {
		t.Fatal("SSE parser did not fill output channel")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("SSE parser stayed blocked on full output after cancellation")
	}
}
