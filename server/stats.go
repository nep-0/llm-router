package server

import (
	"encoding/json"
	"llm-router/client"
	"net/http"
)

// StatsResponse is the JSON envelope returned for performance stats
type StatsResponse struct {
	Object string                       `json:"object"`
	Data   map[string]client.ModelStats `json:"data"`
}

// HandleStatsRequest returns an http.HandlerFunc that serves performance stats
func (s *Server) HandleStatsRequest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get stats from global stats tracker
		stats := client.GlobalStats.GetStats()

		resp := StatsResponse{
			Object: "performance_stats",
			Data:   stats,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
