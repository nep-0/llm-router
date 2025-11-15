package server

import (
	"context"
	"llm-router/client"
	"log/slog"
	"net/http"

	"github.com/sashabaranov/go-openai"
)

type Server struct {
	APIKey              string
	Logger              *slog.Logger
	handleRequest       func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionResponse, error)
	handleStreamRequest func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionStream, error)
	handleModels        func() []ModelInfo
}

func NewServer(apiKey string, logger *slog.Logger,
	handleRequest func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionResponse, error),
	handleStreamRequest func(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionStream, error),
	handleModels func() []ModelInfo,
) *Server {
	return &Server{
		APIKey:              apiKey,
		Logger:              logger,
		handleRequest:       handleRequest,
		handleStreamRequest: handleStreamRequest,
		handleModels:        handleModels,
	}
}

func (s *Server) ListenAndServe(addr string) {
	s.Logger.Info("Server listening", slog.String("address", addr))
	http.HandleFunc("/v1/chat/completions", compressionMiddleware(s.HandleCompletionsRequest))
	// expose models list
	if s.handleModels != nil {
		http.HandleFunc("/v1/models", compressionMiddleware(s.HandleModelsRequest(s.handleModels)))
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	http.ListenAndServe(addr, nil)
}
