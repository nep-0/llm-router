package app

import (
	"llm-router/client"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestGetClientWithMultipleModelsFromSameProvider(t *testing.T) {
	// Setup: Create test clients with usage tracking
	config1 := openai.DefaultConfig("key1")
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(config1), 0, 0)

	config2 := openai.DefaultConfig("key2")
	kc2 := client.NewKeyClient("key2", openai.NewClientWithConfig(config2), 0, 0)

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
		{Weight: 1, Provider: "openai", Name: "gpt-4-turbo-preview"},
		{Weight: 1, Provider: "openai", Name: "gpt-4"},
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
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(openai.DefaultConfig("key1")), 0, 0)
	kc2 := client.NewKeyClient("key2", openai.NewClientWithConfig(openai.DefaultConfig("key2")), 0, 0)
	kc3 := client.NewKeyClient("key3", openai.NewClientWithConfig(openai.DefaultConfig("key3")), 0, 0)

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
		{Weight: 1, Provider: "openai", Name: "gpt-4-turbo-preview"},
		{Weight: 1, Provider: "openai", Name: "gpt-4"},
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

func TestWeightedBalancing(t *testing.T) {
	// Create multiple API keys
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(openai.DefaultConfig("key1")), 0, 0)
	kc2 := client.NewKeyClient("key2", openai.NewClientWithConfig(openai.DefaultConfig("key2")), 0, 0)

	app := &App{
		clients: map[string]*client.ProviderClient{
			"openai": {
				ProviderName: "openai",
				KeyClients:   []*client.KeyClient{kc1, kc2},
			},
		},
	}

	// Set up usage where model-a has 100 tokens on kc1, model-b has 50 tokens on kc1
	// Also set non-zero usage on kc2 to avoid it being selected
	kc1.IncrementUsage("model-a", 100)
	kc1.IncrementUsage("model-b", 50)
	kc2.IncrementUsage("model-a", 200)
	kc2.IncrementUsage("model-b", 200)

	// Test with equal weights (weight=1)
	models := []*Model{
		{Weight: 1, Provider: "openai", Name: "model-a"},
		{Weight: 1, Provider: "openai", Name: "model-b"},
	}

	provider, model, selectedClient := app.getClient(models)

	// Should select model-b with kc1 (weighted usage: 50*1=50, lower than model-a's 100*1=100)
	if provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", provider)
	}
	if model != "model-b" {
		t.Errorf("Expected model 'model-b', got '%s'", model)
	}
	if selectedClient != kc1 {
		t.Errorf("Expected kc1 to be selected")
	}

	// Test with different weights
	// model-a has weight 1, model-b has weight 2
	// kc1: model-a usage=100, model-b usage=50
	// Weighted: model-a=100*1=100, model-b=50*2=100 (tied, model-a selected first)
	// kc2: model-a usage=200, model-b usage=200
	// Weighted: model-a=200*1=200, model-b=200*2=400
	models = []*Model{
		{Weight: 1, Provider: "openai", Name: "model-a"},
		{Weight: 2, Provider: "openai", Name: "model-b"},
	}

	provider, model, selectedClient = app.getClient(models)

	// With weight=2 for model-b, kc1's weighted usage is 100 for model-a and 100 for model-b
	// Should select either model-a or model-b on kc1 (both have weighted usage 100)
	if selectedClient != kc1 {
		t.Errorf("Expected kc1 to be selected (lowest weighted usage)")
	}
}

func TestWeightedBalancingPrioritizesHigherWeight(t *testing.T) {
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(openai.DefaultConfig("key1")), 0, 0)

	app := &App{
		clients: map[string]*client.ProviderClient{
			"openai": {
				ProviderName: "openai",
				KeyClients:   []*client.KeyClient{kc1},
			},
		},
	}

	// Model A: 100 tokens, weight 1 → effective usage = 100
	// Model B: 100 tokens, weight 2 → effective usage = 200
	// Model C: 100 tokens, weight 3 → effective usage = 300
	kc1.IncrementUsage("model-a", 100)
	kc1.IncrementUsage("model-b", 100)
	kc1.IncrementUsage("model-c", 100)

	models := []*Model{
		{Weight: 1, Provider: "openai", Name: "model-a"},
		{Weight: 2, Provider: "openai", Name: "model-b"},
		{Weight: 3, Provider: "openai", Name: "model-c"},
	}

	_, model, _ := app.getClient(models)

	// Should select model-a because it has the lowest weighted usage (100*1=100)
	if model != "model-a" {
		t.Errorf("Expected model 'model-a' (lowest weighted usage), got '%s'", model)
	}
}

func TestWeightedBalancingDistributesLoad(t *testing.T) {
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(openai.DefaultConfig("key1")), 0, 0)

	app := &App{
		clients: map[string]*client.ProviderClient{
			"openai": {
				ProviderName: "openai",
				KeyClients:   []*client.KeyClient{kc1},
			},
		},
	}

	// Scenario: weight 2 means "use this half as often"
	// If model-expensive has weight 2 and model-cheap has weight 1,
	// after equal token usage, model-cheap should be selected
	kc1.IncrementUsage("model-expensive", 100) // weight 2 → effective 200
	kc1.IncrementUsage("model-cheap", 150)     // weight 1 → effective 150

	models := []*Model{
		{Weight: 2, Provider: "openai", Name: "model-expensive"},
		{Weight: 1, Provider: "openai", Name: "model-cheap"},
	}

	_, model, _ := app.getClient(models)

	// Should select model-cheap (weighted usage 150 < 200)
	if model != "model-cheap" {
		t.Errorf("Expected model 'model-cheap' (weighted usage 150 < 200), got '%s'", model)
	}

	// Now increment model-cheap to 201 tokens (weighted: 201)
	kc1.IncrementUsage("model-cheap", 51)

	_, model, _ = app.getClient(models)

	// Should now select model-expensive (weighted usage 200 < 201)
	if model != "model-expensive" {
		t.Errorf("Expected model 'model-expensive' after rebalancing, got '%s'", model)
	}
}

func TestWeightedBalancingAcrossMultipleKeys(t *testing.T) {
	kc1 := client.NewKeyClient("key1", openai.NewClientWithConfig(openai.DefaultConfig("key1")), 0, 0)
	kc2 := client.NewKeyClient("key2", openai.NewClientWithConfig(openai.DefaultConfig("key2")), 0, 0)
	kc3 := client.NewKeyClient("key3", openai.NewClientWithConfig(openai.DefaultConfig("key3")), 0, 0)

	app := &App{
		clients: map[string]*client.ProviderClient{
			"openai": {
				ProviderName: "openai",
				KeyClients:   []*client.KeyClient{kc1, kc2, kc3},
			},
		},
	}

	// Setup: model-a (weight 1), model-b (weight 2)
	// kc1: model-a=100, model-b=50 → weighted: 100, 100
	// kc2: model-a=50, model-b=100 → weighted: 50, 200
	// kc3: model-a=75, model-b=30 → weighted: 75, 60
	kc1.IncrementUsage("model-a", 100)
	kc1.IncrementUsage("model-b", 50)
	kc2.IncrementUsage("model-a", 50)
	kc2.IncrementUsage("model-b", 100)
	kc3.IncrementUsage("model-a", 75)
	kc3.IncrementUsage("model-b", 30)

	models := []*Model{
		{Weight: 1, Provider: "openai", Name: "model-a"},
		{Weight: 2, Provider: "openai", Name: "model-b"},
	}

	provider, model, selectedClient := app.getClient(models)

	// Minimum weighted usage:
	// kc1+model-a: 100*1=100
	// kc1+model-b: 50*2=100
	// kc2+model-a: 50*1=50 ← minimum!
	// kc2+model-b: 100*2=200
	// kc3+model-a: 75*1=75
	// kc3+model-b: 30*2=60

	if provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", provider)
	}
	if model != "model-a" {
		t.Errorf("Expected model 'model-a' (lowest weighted usage), got '%s'", model)
	}
	if selectedClient != kc2 {
		t.Errorf("Expected kc2 to be selected (weighted usage 50)")
	}
}
