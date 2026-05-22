package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/hooks"
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
	for i := range profiles {
		p := &profiles[i]
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

func (s *Server) handleDaemonStatus(w http.ResponseWriter, r *http.Request) {
	if s.deps.Daemon == nil {
		writeJSON(w, http.StatusOK, daemonStatusResponse{
			Mode:    "foreground",
			Status:  "running",
			Version: s.version,
			Running: false,
		})
		return
	}
	status, err := s.deps.Daemon.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, daemonStatusFromContract(&status, s.version))
}

func (s *Server) handleHooksStatus(w http.ResponseWriter, r *http.Request) {
	if s.deps.Hooks == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("hooks status provider unavailable"))
		return
	}
	results, err := s.deps.Hooks.Status(r.Context(), hooksStatusOptions(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func hooksStatusOptions(r *http.Request) hooks.StatusOptions {
	return hooks.StatusOptions{Profile: r.URL.Query().Get("profile")}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dur, err := parseSinceParam(q.Get("since"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	limit, err := parseLimitParam(q.Get("limit"), 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	now := time.Now().UTC()
	rows, err := s.deps.Store.QuerySessions(r.Context(), contracts.SessionQuery{
		Profile: q.Get("profile"),
		Status:  q.Get("status"),
		Since:   now.Add(-dur),
		Limit:   limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	slices.SortFunc(rows, func(a, b contracts.SessionTelemetry) int {
		return b.LastSeenAt.Compare(a.LastSeenAt)
	})
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleHeadroom(w http.ResponseWriter, r *http.Request) {
	if s.deps.Profiles == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("profile lister unavailable"))
		return
	}
	profiles, err := s.deps.Profiles.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	unavailable := map[string]string{}
	if s.deps.Ingestor != nil {
		unavailable, err = s.deps.Ingestor.IngestHeadroomProfiles(r.Context(), profiles)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	evaluator := s.deps.Headroom
	if evaluator == nil {
		evaluator = headroom.Evaluator{Store: s.deps.Store, Pricing: s.deps.Pricing}
	}
	result, err := evaluator.Evaluate(r.Context(), profiles, headroom.Options{
		UnavailableReasons: unavailable,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseSinceParam(s string) (time.Duration, error) {
	if s == "" {
		return 24 * time.Hour, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("since must be > 0")
		}
		return d, nil
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			if n <= 0 {
				return 0, fmt.Errorf("since must be > 0")
			}
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("invalid since: %q", s)
}

func parseLimitParam(s string, defaultLimit int) (int, error) {
	if s == "" {
		return defaultLimit, nil
	}
	limit, err := strconv.Atoi(s)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("invalid limit: %q", s)
	}
	if limit > 200 {
		return 200, nil
	}
	return limit, nil
}

type daemonStatusResponse struct {
	Mode            string     `json:"mode"`
	Status          string     `json:"status"`
	PID             int        `json:"pid,omitempty"`
	Version         string     `json:"version"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	Port            int        `json:"port,omitempty"`
	URL             string     `json:"url,omitempty"`
	DBPath          string     `json:"db_path,omitempty"`
	LogPath         string     `json:"log_path,omitempty"`
	ExecutablePath  string     `json:"executable_path,omitempty"`
	ProfilesWatched int        `json:"profiles_watched,omitempty"`
	Running         bool       `json:"running"`
}

func daemonStatusFromContract(status *contracts.DaemonStatus, fallbackVersion string) daemonStatusResponse {
	mode := "offline"
	state := "offline"
	if status.Running {
		mode = "daemon"
		state = "running"
	} else if status.PID > 0 {
		mode = "daemon"
		state = "starting"
	}
	version := status.Version
	if version == "" {
		version = fallbackVersion
	}
	var started *time.Time
	if !status.StartedAt.IsZero() {
		t := status.StartedAt
		started = &t
	}
	return daemonStatusResponse{
		Mode:            mode,
		Status:          state,
		PID:             status.PID,
		Version:         version,
		StartedAt:       started,
		Port:            status.Port,
		URL:             status.URL,
		DBPath:          status.DBPath,
		LogPath:         status.LogPath,
		ExecutablePath:  status.ExecutablePath,
		ProfilesWatched: status.ProfilesWatched,
		Running:         status.Running,
	}
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
