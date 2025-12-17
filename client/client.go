package client

import (
	"context"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
)

type ProviderClient struct {
	ProviderName string
	KeyClients   []*KeyClient
}

type KeyClient struct {
	APIKey       string
	ProviderName string           // provider name for stats tracking
	modelUsage   map[string]int64 // per-model usage tracking
	usageMutex   sync.RWMutex     // protects modelUsage map
	Client       *openai.Client

	errorPenalty   int64
	requestPenalty int64
}

// NewKeyClient creates a new KeyClient with initialized model usage map
func NewKeyClient(apiKey string, providerName string, client *openai.Client, errorPenalty int64, requestPenalty int64) *KeyClient {
	return &KeyClient{
		APIKey:         apiKey,
		ProviderName:   providerName,
		modelUsage:     make(map[string]int64),
		Client:         client,
		errorPenalty:   errorPenalty,
		requestPenalty: requestPenalty,
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
	provider  string
	// Different providers report usage in very different ways.
	// In case of reporting multiple times, we track usage here to avoid double counting.
	usage int64
	// Performance tracking
	startTime      time.Time
	firstTokenTime time.Duration
	firstTokenSent bool
}

// Recv receives the next stream chunk and tracks usage
func (w *ChatCompletionStream) Recv() (openai.ChatCompletionStreamResponse, error) {
	resp, err := w.stream.Recv()
	if err != nil {
		// On EOF or error, record final performance stats if we have usage data
		if w.usage > 0 {
			totalDuration := time.Since(w.startTime)
			GlobalStats.RecordRequest(w.provider, w.model, w.firstTokenTime, w.usage, totalDuration)
		}
		return resp, err
	}

	// Track first token time
	if !w.firstTokenSent {
		// Check if this chunk contains actual content
		if len(resp.Choices) > 0 && (resp.Choices[0].Delta.Content != "" || resp.Choices[0].Delta.ReasoningContent != "") {
			w.firstTokenTime = time.Since(w.startTime)
			w.firstTokenSent = true
		}
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
	kc.IncrementUsage(req.Model, kc.requestPenalty)

	startTime := time.Now()
	resp, err := kc.Client.CreateChatCompletion(ctx, req)
	totalDuration := time.Since(startTime)

	if err != nil {
		kc.IncrementUsage(req.Model, kc.errorPenalty)
		return nil, err
	}
	kc.IncrementUsage(req.Model, int64(resp.Usage.TotalTokens))

	// Record performance stats (for non-streaming, first token time = total time)
	GlobalStats.RecordRequest(kc.ProviderName, req.Model, totalDuration, int64(resp.Usage.TotalTokens), totalDuration)

	wrapped := &ChatCompletionResponse{
		ChatCompletionResponse: resp,
	}
	return wrapped, nil
}

// ChatCompletionStream wraps the CreateChatCompletionStream method and tracks usage
func (kc *KeyClient) ChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*ChatCompletionStream, error) {
	kc.IncrementUsage(req.Model, kc.requestPenalty)

	startTime := time.Now()
	stream, err := kc.Client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		kc.IncrementUsage(req.Model, kc.errorPenalty)
		return nil, err
	}

	wrapper := &ChatCompletionStream{
		stream:         stream,
		keyClient:      kc,
		model:          req.Model,
		provider:       kc.ProviderName,
		usage:          0,
		startTime:      startTime,
		firstTokenSent: false,
	}

	return wrapper, nil
}
