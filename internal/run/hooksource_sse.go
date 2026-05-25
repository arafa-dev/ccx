package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// OpenSSE subscribes to a loopback /api/recommendations/live SSE endpoint and
// returns parsed recommendation events until ctx is canceled or the stream ends.
func OpenSSE(ctx context.Context, rawURL string) (<-chan contracts.RecommendationEvent, error) {
	if err := validateLoopbackHTTPURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating SSE request: %w", err)
	}
	//nolint:bodyclose // The streaming body is closed by the reader goroutine.
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opening SSE stream: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("opening SSE stream: status %d", res.StatusCode)
	}

	events := make(chan contracts.RecommendationEvent, 16)
	go func() {
		defer close(events)
		defer func() { _ = res.Body.Close() }()
		readRecommendationSSE(ctx, res.Body, events)
	}()
	return events, nil
}

func validateLoopbackHTTPURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parsing SSE URL: %w", err)
	}
	if parsed.Scheme != "http" {
		return fmt.Errorf("opening SSE stream: unsupported scheme %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("opening SSE stream: missing host")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("opening SSE stream: non-loopback host %q", host)
	}
	return nil
}

type sseReader interface {
	Read(p []byte) (int, error)
}

func readRecommendationSSE(ctx context.Context, r sseReader, out chan<- contracts.RecommendationEvent) {
	scanner := bufio.NewScanner(r)
	eventName := ""
	var data strings.Builder
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return
		}
		line := scanner.Text()
		if line == "" {
			if !emitRecommendationEvent(ctx, eventName, data.String(), out) {
				return
			}
			eventName = ""
			data.Reset()
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func emitRecommendationEvent(ctx context.Context, eventName, data string, out chan<- contracts.RecommendationEvent) bool {
	if data == "" || eventName != "recommendation" {
		return true
	}
	var ev contracts.RecommendationEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return true
	}
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
