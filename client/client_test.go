package client

import (
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestPerModelUsageTracking(t *testing.T) {
	// Create a new KeyClient
	config := openai.DefaultConfig("test-key")
	client := openai.NewClientWithConfig(config)
	kc := NewKeyClient("test-key", client, 0, 0)

	// Test initial usage is 0 for any model
	if usage := kc.Usage("gpt-4"); usage != 0 {
		t.Errorf("Expected initial usage for gpt-4 to be 0, got %d", usage)
	}
	if usage := kc.Usage("gpt-3.5-turbo"); usage != 0 {
		t.Errorf("Expected initial usage for gpt-3.5-turbo to be 0, got %d", usage)
	}

	// Increment usage for gpt-4
	kc.IncrementUsage("gpt-4", 100)
	if usage := kc.Usage("gpt-4"); usage != 100 {
		t.Errorf("Expected usage for gpt-4 to be 100, got %d", usage)
	}
	if usage := kc.Usage("gpt-3.5-turbo"); usage != 0 {
		t.Errorf("Expected usage for gpt-3.5-turbo to remain 0, got %d", usage)
	}

	// Increment usage for gpt-3.5-turbo
	kc.IncrementUsage("gpt-3.5-turbo", 50)
	if usage := kc.Usage("gpt-3.5-turbo"); usage != 50 {
		t.Errorf("Expected usage for gpt-3.5-turbo to be 50, got %d", usage)
	}
	if usage := kc.Usage("gpt-4"); usage != 100 {
		t.Errorf("Expected usage for gpt-4 to remain 100, got %d", usage)
	}

	// Increment usage for gpt-4 again
	kc.IncrementUsage("gpt-4", 25)
	if usage := kc.Usage("gpt-4"); usage != 125 {
		t.Errorf("Expected usage for gpt-4 to be 125, got %d", usage)
	}

	// Test that different models have independent usage tracking
	kc.IncrementUsage("gpt-4-turbo", 200)
	if usage := kc.Usage("gpt-4-turbo"); usage != 200 {
		t.Errorf("Expected usage for gpt-4-turbo to be 200, got %d", usage)
	}
	if usage := kc.Usage("gpt-4"); usage != 125 {
		t.Errorf("Expected usage for gpt-4 to remain 125, got %d", usage)
	}
	if usage := kc.Usage("gpt-3.5-turbo"); usage != 50 {
		t.Errorf("Expected usage for gpt-3.5-turbo to remain 50, got %d", usage)
	}
}

func TestConcurrentUsageTracking(t *testing.T) {
	config := openai.DefaultConfig("test-key")
	client := openai.NewClientWithConfig(config)
	kc := NewKeyClient("test-key", client, 0, 0)

	// Test concurrent increments
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			kc.IncrementUsage("gpt-4", 1)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	if usage := kc.Usage("gpt-4"); usage != 100 {
		t.Errorf("Expected concurrent usage for gpt-4 to be 100, got %d", usage)
	}
}
