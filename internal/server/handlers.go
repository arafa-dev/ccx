package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.version})
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profiles, err := s.deps.Profiles.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	start := time.Now().UTC().Truncate(24 * time.Hour)
	out := make([]map[string]any, 0, len(profiles))
	for _, p := range profiles {
		rows, err := s.deps.Store.QueryUsage(ctx, contracts.UsageQuery{
			Profile: p.Name,
			Range:   contracts.TimeRange{Start: start, End: start.Add(24 * time.Hour)},
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("query usage for profile %q: %w", p.Name, err))
			return
		}
		usage, cost, err := aggregate(s.deps.Pricing, rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("pricing for profile %q: %w", p.Name, err))
			return
		}
		out = append(out, map[string]any{
			"name":         p.Name,
			"config_dir":   p.ConfigDir,
			"label":        p.Label,
			"color":        p.Color,
			"created_at":   p.CreatedAt,
			"last_used_at": p.LastUsedAt,
			"today": map[string]any{
				"usage":         usage,
				"estimated_usd": cost,
			},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	dur, err := parseSinceParam(q.Get("since"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	now := time.Now().UTC()
	rows, err := s.deps.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: q.Get("profile"),
		Project: q.Get("project"),
		Range:   contracts.TimeRange{Start: now.Add(-dur), End: now},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for i := range rows {
		cost, err := s.deps.Pricing.Cost(rows[i].Model, rows[i].Day, rows[i].Usage)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("pricing error: %w", err))
			return
		}
		rows[i].EstimatedUSD = cost
	}
	total := totalUsage(rows)
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "total": total})
}

func (s *Server) handleUsageLive(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			rows, err := s.deps.Store.QueryUsage(r.Context(), contracts.UsageQuery{
				Range: contracts.TimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
			})
			if err != nil {
				continue
			}
			b, _ := json.Marshal(rows)
			_, _ = fmt.Fprintf(w, "event: usage\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func parseSinceParam(s string) (time.Duration, error) {
	if s == "" {
		return 24 * time.Hour, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("invalid since: %q", s)
}

func aggregate(p contracts.PricingTable, rows []contracts.UsageRow) (usage contracts.Usage, cost float64, err error) {
	for _, r := range rows {
		usage = usage.Add(r.Usage)
		rowCost, err := p.Cost(r.Model, r.Day, r.Usage)
		if err != nil {
			return usage, cost, err
		}
		cost += rowCost
	}
	return usage, cost, nil
}

func totalUsage(rows []contracts.UsageRow) map[string]any {
	var (
		u contracts.Usage
		c float64
	)
	for _, r := range rows {
		u = u.Add(r.Usage)
		c += r.EstimatedUSD
	}
	return map[string]any{"usage": u, "estimated_usd": c}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
