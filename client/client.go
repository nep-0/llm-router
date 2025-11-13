package client

import (
	"context"
	"sync/atomic"

	"github.com/sashabaranov/go-openai"
)

type ProviderClient struct {
	ProviderName string
	KeyClients   []*KeyClient
}

type KeyClient struct {
	APIKey string
	usage  int64
	Client *openai.Client
}

// IncrementUsage increases the usage count for the KeyClient
func (kc *KeyClient) IncrementUsage(in int64) {
	atomic.AddInt64(&kc.usage, in)
}

// Usage returns the current usage count for the KeyClient
func (kc *KeyClient) Usage() int64 {
	return atomic.LoadInt64(&kc.usage)
}

// ChatCompletionResponse wraps the OpenAI response
type ChatCompletionResponse struct {
	openai.ChatCompletionResponse
}

// ChatCompletionStream wraps the OpenAI stream to track usage
type ChatCompletionStream struct {
	stream    *openai.ChatCompletionStream
	keyClient *KeyClient
}

// Recv receives the next stream chunk and tracks usage
func (w *ChatCompletionStream) Recv() (openai.ChatCompletionStreamResponse, error) {
	resp, err := w.stream.Recv()
	if err != nil {
		return resp, err
	}

	// Increment usage if provided in the response
	if resp.Usage != nil {
		w.keyClient.IncrementUsage(int64(resp.Usage.TotalTokens))
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
	kc.IncrementUsage(int64(resp.Usage.TotalTokens))

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
	}

	return wrapper, nil
}
