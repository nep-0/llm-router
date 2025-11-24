package client

import (
	"context"
	"sync"

	"github.com/sashabaranov/go-openai"
)

type ProviderClient struct {
	ProviderName string
	KeyClients   []*KeyClient
}

type KeyClient struct {
	APIKey     string
	modelUsage map[string]int64 // per-model usage tracking
	usageMutex sync.RWMutex     // protects modelUsage map
	Client     *openai.Client
}

// NewKeyClient creates a new KeyClient with initialized model usage map
func NewKeyClient(apiKey string, client *openai.Client) *KeyClient {
	return &KeyClient{
		APIKey:     apiKey,
		modelUsage: make(map[string]int64),
		Client:     client,
	}
}

// IncrementUsage increases the usage count for a specific model
func (kc *KeyClient) IncrementUsage(model string, tokens int64) {
	kc.usageMutex.Lock()
	defer kc.usageMutex.Unlock()
	kc.modelUsage[model] += tokens
}

// Usage returns the current usage count for a specific model
func (kc *KeyClient) Usage(model string) int64 {
	kc.usageMutex.RLock()
	defer kc.usageMutex.RUnlock()
	return kc.modelUsage[model]
}

// ChatCompletionResponse wraps the OpenAI response
type ChatCompletionResponse struct {
	openai.ChatCompletionResponse
}

// ChatCompletionStream wraps the OpenAI stream to track usage
type ChatCompletionStream struct {
	stream    *openai.ChatCompletionStream
	keyClient *KeyClient
	model     string
	// Different providers report usage in very different ways.
	// In case of reporting multiple times, we track usage here to avoid double counting.
	usage int64
}

// Recv receives the next stream chunk and tracks usage
func (w *ChatCompletionStream) Recv() (openai.ChatCompletionStreamResponse, error) {
	resp, err := w.stream.Recv()
	if err != nil {
		return resp, err
	}

	// Empty response choices
	finish := len(resp.Choices) == 0

	// Response choice with finish reason
	finish = finish || resp.Choices[0].FinishReason != ""

	// Response choice with empty content
	finish = finish || resp.Choices[0].Delta.Content+resp.Choices[0].Delta.ReasoningContent == ""

	// Increment usage if finish and usage info is available
	if finish && resp.Usage != nil {
		delta := int64(resp.Usage.TotalTokens) - w.usage
		if delta > 0 {
			w.keyClient.IncrementUsage(w.model, delta)
			w.usage += delta
		}
	}

	return resp, nil
}

// Close closes the underlying stream
func (w *ChatCompletionStream) Close() error {
	return w.stream.Close()
}

// ChatCompletion wraps the CreateChatCompletion method and increments usage
func (kc *KeyClient) ChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (*ChatCompletionResponse, error) {
	resp, err := kc.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}
	kc.IncrementUsage(req.Model, int64(resp.Usage.TotalTokens))

	wrapped := &ChatCompletionResponse{
		ChatCompletionResponse: resp,
	}
	return wrapped, nil
}

// ChatCompletionStream wraps the CreateChatCompletionStream method and tracks usage
func (kc *KeyClient) ChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*ChatCompletionStream, error) {
	stream, err := kc.Client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	wrapper := &ChatCompletionStream{
		stream:    stream,
		keyClient: kc,
		model:     req.Model,
		usage:     0,
	}

	return wrapper, nil
}
