package app

import (
	"llm-router/client"
	"llm-router/config"
	"llm-router/server"

	"github.com/sashabaranov/go-openai"
)

// getGroups initializes groups based on the configuration
func getGroups(cfg *config.Config) []*Group {
	groups := make([]*Group, 0)
	for _, cfgGroup := range cfg.Groups {
		group := &Group{
			Name:   cfgGroup.Name,
			Models: make([]*Model, 0),
		}
		for _, cfgModel := range cfgGroup.Models {
			model := &Model{
				Weight:   cfgModel.Weight,
				Provider: cfgModel.Provider,
				Name:     cfgModel.Name,
			}
			group.Models = append(group.Models, model)
		}
		groups = append(groups, group)
	}
	return groups
}

// getProviders initializes providers based on the configuration
func getProviders(cfg *config.Config) []*Provider {
	providers := make([]*Provider, 0)
	for _, cfgProvider := range cfg.Providers {
		provider := &Provider{
			Name:    cfgProvider.Name,
			BaseURL: cfgProvider.BaseURL,
			APIKeys: cfgProvider.APIKeys,
		}
		providers = append(providers, provider)
	}
	return providers
}

// getClients initializes provider clients based on the configuration
func getClients(cfg *config.Config) map[string]*client.ProviderClient {
	clients := make(map[string]*client.ProviderClient)
	for _, provider := range cfg.Providers {
		pClient := &client.ProviderClient{
			ProviderName: provider.Name,
		}
		for _, apiKey := range provider.APIKeys {
			openAIConfig := openai.DefaultConfig(apiKey)
			openAIConfig.BaseURL = provider.BaseURL
			keyClient := &client.KeyClient{
				APIKey: apiKey,
				Client: openai.NewClientWithConfig(openAIConfig),
			}
			pClient.KeyClients = append(pClient.KeyClients, keyClient)
		}
		clients[provider.Name] = pClient
	}
	return clients
}

// getServer creates a new server instance with request handlers
func (a *App) getServer() *server.Server {
	return server.NewServer(
		a.Config.APIKey,
		a.Logger,
		a.HandleRequest,
		a.HandleStreamRequest,
	)
}
