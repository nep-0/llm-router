package app

import (
	"context"
	"fmt"
	"llm-router/client"
	"llm-router/config"
	"llm-router/server"
	"log/slog"
	"os"

	"github.com/sashabaranov/go-openai"
)

type App struct {
	Config    *config.Config
	Logger    *slog.Logger
	Server    *server.Server
	Groups    []*Group
	Providers []*Provider
	clients   map[string]*client.ProviderClient
}

// NewApp initializes the application with configuration, groups, providers, and clients
func NewApp(cfg *config.Config) *App {
	app := &App{
		Config:    cfg,
		Groups:    getGroups(cfg),
		Providers: getProviders(cfg),
		clients:   getClients(cfg),
	}
	app.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app.Server = app.getServer()
	return app
}

// Run starts the server and begins handling requests
func (a *App) Run() {
	a.Logger.Info("Starting LLM Router", slog.Int64("port", a.Config.Port))
	addr := fmt.Sprintf(":%d", a.Config.Port)
	a.Server.ListenAndServe(addr)
}

// HandleRequest processes chat completion requests
func (a *App) HandleRequest(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionResponse, error) {
	provider, model, keyClient, err := a.getClientForGroup(req.Model)
	if err != nil {
		a.Logger.Error("Failed to get client for group", slog.String("group", req.Model), slog.Any("error", err))
		return nil, err
	}
	a.Logger.Info("Routing request", slog.String("provider", provider), slog.String("model", model))

	// Update the request model to the selected model
	req.Model = model
	resp, err := keyClient.ChatCompletion(ctx, req)
	if err != nil {
		a.Logger.Error("ChatCompletion error", slog.Any("error", err))
		return nil, err
	}
	return resp, nil
}

// HandleStreamRequest processes streaming chat completion requests
func (a *App) HandleStreamRequest(ctx context.Context, req openai.ChatCompletionRequest) (*client.ChatCompletionStream, error) {
	provider, model, keyClient, err := a.getClientForGroup(req.Model)
	if err != nil {
		a.Logger.Error("Failed to get client for group", slog.String("group", req.Model), slog.Any("error", err))
		return nil, err
	}
	a.Logger.Info("Routing streaming request", slog.String("provider", provider), slog.String("model", model))

	// Update the request model to the selected model
	req.Model = model
	stream, err := keyClient.ChatCompletionStream(ctx, req)
	if err != nil {
		a.Logger.Error("ChatCompletionStream error", slog.Any("error", err))
		return nil, err
	}
	return stream, nil
}

// getClientForGroup selects the appropriate provider, model, and KeyClient for the given group name
func (a *App) getClientForGroup(groupName string) (provider string, model string, keyClient *client.KeyClient, err error) {
	// Find the models of the group in the config
	var models []*Model
	for _, group := range a.Groups {
		if group.Name == groupName {
			models = group.Models
			break
		}
	}

	if len(models) == 0 {
		return "", "", nil, fmt.Errorf("no models found for group: %s", groupName)
	}

	provider, model, client := a.getClient(models)

	return provider, model, client, nil
}

// getClient selects the KeyClient with the lowest usage for the specific provider/model combination
func (a *App) getClient(models []*Model) (provider string, model string, keyClient *client.KeyClient) {
	minUsage := int64(-1)
	var selectedProvider string
	var selectedModel string
	var selectedClient *client.KeyClient

	// Iterate over all models in the group
	for _, m := range models {
		if pClient, exists := a.clients[m.Provider]; exists {
			for _, kClient := range pClient.KeyClients {
				usage := kClient.Usage(m.Name)
				if minUsage == -1 || usage < minUsage {
					minUsage = usage
					selectedClient = kClient
					selectedProvider = m.Provider
					selectedModel = m.Name
				}
			}
		}
	}
	return selectedProvider, selectedModel, selectedClient
}
