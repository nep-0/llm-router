package server

import (
	"encoding/json"
	"net/http"
)

// ModelInfo represents the minimal model metadata returned by the /v1/models endpoint
type ModelInfo struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	OwnedBy string                 `json:"owned_by"`
	Extra   map[string]interface{} `json:"-"`
}

// ModelsListResponse is the JSON envelope returned for list models
type ModelsListResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// HandleModelsRequest returns an http.HandlerFunc that serves the models list.
// The provided modelsFunc should return the list of ModelInfo to expose.
func (s *Server) HandleModelsRequest(modelsFunc func() []ModelInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Build response
		models := modelsFunc()
		resp := ModelsListResponse{
			Object: "list",
			Data:   models,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
