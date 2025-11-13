/*
 * Copyright (c) 2024 Keith Coleman
 * Copyright (c) 2025 Jeff
 *
 * This file is based on code from:
 * https://github.com/kcolemangt/llm-router/blob/859fea37d5691b34a6e3734ea273544c419f6ce2/handler/handler.go
 * Original commit: 859fea37d5691b34a6e3734ea273544c419f6ce2
 *
 * SPDX-License-Identifier: MIT
 */

package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"llm-router/client"
	"llm-router/utils"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sashabaranov/go-openai"
)

type Server struct {
	APIKey              string
	Logger              *slog.Logger
	handleRequest       func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionResponse, error)
	handleStreamRequest func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionStream, error)
}

func NewServer(apiKey string, logger *slog.Logger,
	handleRequest func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionResponse, error),
	handleStreamRequest func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionStream, error),
) *Server {
	return &Server{
		APIKey:              apiKey,
		Logger:              logger,
		handleRequest:       handleRequest,
		handleStreamRequest: handleStreamRequest,
	}
}

func (s *Server) ListenAndServe(addr string) {
	s.Logger.Info("Server listening", slog.String("address", addr))
	http.HandleFunc("/v1/chat/completions", s.HandleRequest)
	http.ListenAndServe(addr, nil)
}

// HandleRequest is the main HTTP handler function that processes incoming requests
func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Check if this is likely a streaming request
	isStreaming := false
	if r.URL.Path == "/v1/chat/completions" && r.Method == "POST" {
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			// Read the first 1024 bytes to check for stream parameter
			// without consuming the entire body
			peeked := make([]byte, 1024)
			n, _ := r.Body.Read(peeked)
			if n > 0 {
				peeked = peeked[:n]
				isStreaming = strings.Contains(string(peeked), "\"stream\":true")
				// Restore the body
				combinedReader := io.MultiReader(bytes.NewReader(peeked), r.Body)
				r.Body = io.NopCloser(combinedReader)

				// Update ContentLength to account for the peek operation
				if r.ContentLength > 0 {
					r.ContentLength = int64(n) + r.ContentLength
				}
			}
		}
	}

	// Create a response recorder to capture the response
	recorder := utils.NewResponseRecorder(w)

	// Log the full incoming request if debug is enabled
	var reqBody string
	if r.Body != nil {
		// For streaming, use a more careful approach to draining the body
		if isStreaming {
			r.Body, reqBody = utils.DrainAndCapture(r.Body, isStreaming)
		} else {
			r.Body, reqBody = utils.DrainBody(r.Body)
		}

		// Ensure ContentLength is set correctly after draining
		if r.ContentLength > 0 && !isStreaming {
			bodyBytes := []byte(reqBody)
			r.ContentLength = int64(len(bodyBytes))
		}

		s.Logger.Debug("Incoming request",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Bool("streaming", isStreaming))
		utils.LogRequestResponse(s.Logger, r, nil, reqBody, "")
	}

	// Special handling for OPTIONS requests (CORS preflight)
	if r.Method == "OPTIONS" {
		s.Logger.Debug("Handling OPTIONS request for CORS preflight")

		// Get the request headers
		reqHeaders := r.Header.Get("Access-Control-Request-Headers")
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		// Log the request method requested in preflight
		if reqMethod := r.Header.Get("Access-Control-Request-Method"); reqMethod != "" {
			s.Logger.Debug("Preflight requested method", slog.String("method", reqMethod))
		}

		// Set CORS headers for OPTIONS requests
		recorder.Header().Set("Access-Control-Allow-Origin", origin)
		recorder.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")

		if reqHeaders != "" {
			recorder.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		} else {
			recorder.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
		}

		recorder.Header().Set("Access-Control-Allow-Credentials", "true")
		recorder.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
		recorder.Header().Set("Content-Type", "text/plain")
		recorder.Header().Set("Content-Length", "0")
		recorder.Header().Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")

		// Return 204 No Content for OPTIONS requests
		recorder.WriteHeader(http.StatusNoContent)

		// Log the response
		s.logResponse(s.Logger, recorder)
		return
	}
	// Authenticate the request - only for non-OPTIONS requests
	authHeader := r.Header.Get("Authorization")
	expectedAuthHeader := "Bearer " + s.APIKey

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(expectedAuthHeader)) != 1 {
		s.Logger.Warn("Invalid or missing API key",
			slog.String("receivedAuthHeader", utils.RedactAuthorization(authHeader)),
			slog.String("expectedAuthHeader", utils.RedactAuthorization(expectedAuthHeader)))
		http.Error(recorder, "Invalid or missing API key", http.StatusUnauthorized)

		// Log the response
		s.logResponse(s.Logger, recorder)
		return
	}
	s.Logger.Info("API key validated successfully",
		slog.String("Authorization", utils.RedactAuthorization(authHeader)))

	// Process specific API endpoint logic if applicable
	if r.URL.Path == "/v1/chat/completions" && r.Method == "POST" {
		s.handleChatCompletions(recorder, r)
	} else {
		s.Logger.Info("No handler for this endpoint",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method))
		http.Error(recorder, "Not found", http.StatusNotFound)
	}

	// Log the response
	s.logResponse(s.Logger, recorder)
}

// logResponse logs the details of the HTTP response
func (s *Server) logResponse(logger *slog.Logger, recorder *utils.ResponseRecorder) {
	// Log response status and headers
	logger.Debug("Response details",
		slog.Int("status", recorder.StatusCode),
		slog.Any("headers", recorder.Header()),
		slog.String("body", recorder.GetBody()))
}

// handleChatCompletions processes specific logic for the chat completions endpoint
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var chatReq map[string]any
	if err := json.Unmarshal(body, &chatReq); err != nil {
		http.Error(w, "Error unmarshalling request body", http.StatusInternalServerError)
		return
	}

	modelName, ok := chatReq["model"].(string)
	if !ok {
		http.Error(w, "Model key missing or not a string", http.StatusBadRequest)
		return
	}

	if stream, ok := chatReq["stream"].(bool); ok && stream {
		s.Logger.Info("Incoming streaming request for model(group)", slog.String("model", modelName))

		// Parse the full request
		var req openai.ChatCompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Error parsing request", http.StatusBadRequest)
			return
		}

		stream, err := s.handleStreamRequest(r.Context(), req)
		if err != nil {
			http.Error(w, "Error handling streaming request: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer stream.Close()

		// Set headers for SSE streaming
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Transfer-Encoding", "chunked")

		// Get flusher for immediate streaming
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Stream the responses
		for {
			response, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					// Stream finished successfully
					w.Write([]byte("data: [DONE]\n\n"))
					flusher.Flush()
					return
				}
				s.Logger.Error("Error receiving stream", slog.String("error", err.Error()))
				return
			}

			// Marshal and send the response chunk
			jsonData, err := json.Marshal(response)
			if err != nil {
				s.Logger.Error("Error marshaling stream response", slog.String("error", err.Error()))
				return
			}

			// Write in SSE format
			w.Write([]byte("data: "))
			w.Write(jsonData)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}

	// Handle non-streaming request
	s.Logger.Info("Incoming request for model(group)", slog.String("model", modelName))

	// Parse the full request
	var req openai.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Error parsing request", http.StatusBadRequest)
		return
	}

	// Call the handler
	response, err := s.handleRequest(r.Context(), req)
	if err != nil {
		http.Error(w, "Error handling request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Marshal and send the response
	jsonData, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error marshaling response", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}
