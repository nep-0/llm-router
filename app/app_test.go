package app

import (
	"llm-router/client"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestGetClientWithMultipleModelsFromSameProvider(t *testing.T) {
	// Setup: Create test clients with usage tracking
	config1 := openai.DefaultConfig("key1")
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(config1))

	config2 := openai.DefaultConfig("key2")
	kc2 := client.NewKeyClient("key2", openai.NewClientWithConfig(config2))

	// Create app with test clients
	app := &App{
		clients: map[string]*client.ProviderClient{
			"openai": {
				ProviderName: "openai",
				KeyClients:   []*client.KeyClient{kc1, kc2},
			},
		},
	}

	// Simulate usage: gpt-4 has been heavily used, gpt-4-turbo-preview has not
	kc1.IncrementUsage("gpt-4", 1000)
	kc2.IncrementUsage("gpt-4", 2000)

	// Test: Group with multiple models from same provider (like the gpt-4-turbo group)
	models := []*Model{
		{Provider: "openai", Name: "gpt-4-turbo-preview"},
		{Provider: "openai", Name: "gpt-4"},
	}

	provider, model, selectedClient := app.getClient(models)

	// Should select gpt-4-turbo-preview with kc1 (lowest usage: 0)
	if provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", provider)
	}
	if model != "gpt-4-turbo-preview" {
		t.Errorf("Expected model 'gpt-4-turbo-preview', got '%s'", model)
	}
	if selectedClient != kc1 {
		t.Errorf("Expected kc1 to be selected (lowest usage for gpt-4-turbo-preview: 0)")
	}

	// Now simulate that gpt-4-turbo-preview gets used more than gpt-4 on BOTH keys
	kc1.IncrementUsage("gpt-4-turbo-preview", 1500)
	kc2.IncrementUsage("gpt-4-turbo-preview", 3000)

	provider, model, selectedClient = app.getClient(models)

	// Should now select gpt-4 with kc1 (usage: 1000 < 1500 for kc1, and 1000 < 2000 for kc2)
	if provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", provider)
	}
	if model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", model)
	}
	if selectedClient != kc1 {
		t.Errorf("Expected kc1 to be selected (usage 1000 for gpt-4, which is minimum)")
	}
}

func TestGetClientSelectsLowestUsageAcrossAllCombinations(t *testing.T) {
	// Create multiple API keys
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(openai.DefaultConfig("key1")))
	kc2 := client.NewKeyClient("key2", openai.NewClientWithConfig(openai.DefaultConfig("key2")))
	kc3 := client.NewKeyClient("key3", openai.NewClientWithConfig(openai.DefaultConfig("key3")))

	app := &App{
		clients: map[string]*client.ProviderClient{
			"openai": {
				ProviderName: "openai",
				KeyClients:   []*client.KeyClient{kc1, kc2, kc3},
			},
		},
	}

	// Set different usage for different models across different keys
	// kc1: gpt-4=500, gpt-4-turbo-preview=100
	// kc2: gpt-4=300, gpt-4-turbo-preview=400
	// kc3: gpt-4=200, gpt-4-turbo-preview=600
	kc1.IncrementUsage("gpt-4", 500)
	kc1.IncrementUsage("gpt-4-turbo-preview", 100)
	kc2.IncrementUsage("gpt-4", 300)
	kc2.IncrementUsage("gpt-4-turbo-preview", 400)
	kc3.IncrementUsage("gpt-4", 200)
	kc3.IncrementUsage("gpt-4-turbo-preview", 600)

	models := []*Model{
		{Provider: "openai", Name: "gpt-4-turbo-preview"},
		{Provider: "openai", Name: "gpt-4"},
	}

	provider, model, selectedClient := app.getClient(models)

	// Should select gpt-4-turbo-preview with kc1 (lowest overall: 100)
	if provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", provider)
	}
	if model != "gpt-4-turbo-preview" {
		t.Errorf("Expected model 'gpt-4-turbo-preview', got '%s'", model)
	}
	if selectedClient != kc1 {
		t.Errorf("Expected kc1 to be selected (usage 100 for gpt-4-turbo-preview)")
	}

	// Verify all combinations are considered:
	// kc1+gpt-4-turbo-preview=100 ← minimum
	// kc1+gpt-4=500
	// kc2+gpt-4-turbo-preview=400
	// kc2+gpt-4=300
	// kc3+gpt-4-turbo-preview=600
	// kc3+gpt-4=200
}
