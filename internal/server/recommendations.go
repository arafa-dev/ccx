package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) handleRecommendationsLive(w http.ResponseWriter, r *http.Request) {
	if s.deps.Recommendations == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("recommendations source unavailable"))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}

	events := s.deps.Recommendations.Subscribe(r.Context())

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: recommendation\ndata: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
