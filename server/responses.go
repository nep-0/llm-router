package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"llm-router/utils"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// HandleResponsesRequest provides compatibility with OpenAI's Responses API by
// converting a Responses-style request into a ChatCompletionRequest and
// handling the response format transformation.
func (s *Server) HandleResponsesRequest(w http.ResponseWriter, r *http.Request) {
	// 1. Authentication
	authHeader := r.Header.Get("Authorization")
	expectedAuthHeader := "Bearer " + s.APIKey

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(expectedAuthHeader)) != 1 {
		s.Logger.Warn("Invalid or missing API key",
			slog.String("receivedAuthHeader", utils.RedactAuthorization(authHeader)),
			slog.String("expectedAuthHeader", utils.RedactAuthorization(expectedAuthHeader)))
		http.Error(w, "Invalid or missing API key", http.StatusUnauthorized)
		return
	}

	// 2. Read and Parse Body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var respReq map[string]any
	if err := json.Unmarshal(body, &respReq); err != nil {
		http.Error(w, "Error unmarshalling request body", http.StatusBadRequest)
		return
	}

	// 3. Build a chat-style request map
	chatReq := make(map[string]any)

	// Model: accept either `model` or `model_id`
	if m, ok := respReq["model"].(string); ok && m != "" {
		chatReq["model"] = m
	} else if m, ok := respReq["model_id"].(string); ok && m != "" {
		chatReq["model"] = m
	}

	// Transfer stream flag if present
	isStreaming := false
	if stream, ok := respReq["stream"].(bool); ok {
		chatReq["stream"] = stream
		isStreaming = stream
	}

	// Copy over common fields if present
	copyKeys := []string{"temperature", "max_tokens", "top_p", "n", "stop", "presence_penalty", "frequency_penalty", "user", "logit_bias"}
	for _, k := range copyKeys {
		if v, ok := respReq[k]; ok {
			chatReq[k] = v
		}
	}

	// Convert `input` or `messages` into chat messages
	if msgs, ok := respReq["messages"]; ok {
		// If messages already present, pass through
		chatReq["messages"] = msgs
	} else if input, ok := respReq["input"]; ok {
		// `input` can be a string or an array of strings/objects
		switch v := input.(type) {
		case string:
			chatReq["messages"] = []map[string]any{{"role": "user", "content": v}}
		case []any:
			// Convert each element into a user message (string or JSON-marshaled)
			out := make([]map[string]any, 0, len(v))
			for _, el := range v {
				switch e := el.(type) {
				case string:
					out = append(out, map[string]any{"role": "user", "content": e})
				default:
					// Fallback: marshal element to JSON string
					b, _ := json.Marshal(e)
					out = append(out, map[string]any{"role": "user", "content": string(b)})
				}
			}
			chatReq["messages"] = out
		default:
			// Unknown input type: try to marshal it as JSON and use that as the content
			b, _ := json.Marshal(v)
			chatReq["messages"] = []map[string]any{{"role": "user", "content": string(b)}}
		}
	}

	// Ensure we have a model and messages
	if _, ok := chatReq["model"]; !ok {
		http.Error(w, "Model key missing or not a string", http.StatusBadRequest)
		return
	}
	if _, ok := chatReq["messages"]; !ok {
		http.Error(w, "No input/messages provided", http.StatusBadRequest)
		return
	}

	// 4. Convert to openai.ChatCompletionRequest struct
	chatBytes, err := json.Marshal(chatReq)
	if err != nil {
		http.Error(w, "Error preparing converted request", http.StatusInternalServerError)
		return
	}

	var req openai.ChatCompletionRequest
	if err := json.Unmarshal(chatBytes, &req); err != nil {
		http.Error(w, "Error parsing converted request", http.StatusBadRequest)
		return
	}

	// 5. Handle Request (Streaming or Non-Streaming)
	if isStreaming {
		s.handleResponsesStream(w, r, req)
	} else {
		r.Body = io.NopCloser(bytes.NewReader(chatBytes))
		r.ContentLength = int64(len(chatBytes))
		s.handleChatCompletions(w, r)
	}
}

func (s *Server) handleResponsesStream(w http.ResponseWriter, r *http.Request, req openai.ChatCompletionRequest) {
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Generate IDs
	respID := "resp_" + utils.NewID()
	itemID := "item_" + utils.NewID()
	created := time.Now().Unix()

	// Helper to send event
	sendEvent := func(event map[string]any) error {
		jsonData, err := json.Marshal(event)
		if err != nil {
			return err
		}
		// The error message implies the client expects JSON objects, likely SSE data.
		// Standard OpenAI SSE format is `data: {JSON}\n\n`
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
		return nil
	}

	// 1. Send response.created
	sendEvent(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":      respID,
			"object":  "response",
			"created": created,
			"model":   req.Model,
			"status":  "in_progress",
		},
	})

	// 2. Send response.output_item.added
	sendEvent(map[string]any{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": map[string]any{
			"id":     itemID,
			"object": "realtime.item",
			"type":   "message",
			"status": "in_progress",
			"role":   "assistant",
			"content": []map[string]any{
				{
					"type": "text",
					"text": "",
				},
			},
		},
	})

	// Track content for final event
	var fullContent strings.Builder

	// Stream loop
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			s.Logger.Error("Error receiving stream", slog.String("error", err.Error()))
			// Send error event?
			sendEvent(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "server_error",
					"message": err.Error(),
				},
			})
			return
		}

		if len(resp.Choices) == 0 {
			continue
		}

		content := resp.Choices[0].Delta.Content
		if content != "" {
			fullContent.WriteString(content)
			// 3. Send response.output_text.delta
			sendEvent(map[string]any{
				"type":         "response.output_text.delta",
				"item_id":      itemID,
				"output_index": 0,
				"delta":        content,
			})
		}
	}

	// 4. Send response.output_item.done
	sendEvent(map[string]any{
		"type":         "response.output_item.done",
		"output_index": 0,
		"item": map[string]any{
			"id":     itemID,
			"object": "realtime.item",
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]any{
				{
					"type": "text",
					"text": fullContent.String(),
				},
			},
		},
	})

	// 5. Send response.done
	sendEvent(map[string]any{
		"type": "response.done",
		"response": map[string]any{
			"id":      respID,
			"object":  "response",
			"created": created,
			"model":   req.Model,
			"status":  "completed",
			"output": []map[string]any{
				{
					"id":     itemID,
					"object": "realtime.item",
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []map[string]any{
						{
							"type": "text",
							"text": fullContent.String(),
						},
					},
				},
			},
		},
	})
}
