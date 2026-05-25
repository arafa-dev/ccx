package server

import (
	"fmt"
	"net/http"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request) {
	if s.deps.Quota == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("quota provider unavailable"))
		return
	}
	rows, err := s.deps.Quota.Quota(r.Context(), r.URL.Query().Get("profile"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rows == nil {
		rows = []contracts.ProfileQuota{}
	}
	writeJSON(w, http.StatusOK, rows)
}
