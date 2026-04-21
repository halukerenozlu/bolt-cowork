package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

const (
	openaiAPIURL = "https://api.openai.com/v1/chat/completions"
	openaiMaxTok = 4096
)

// OpenAIProvider calls the OpenAI Chat Completions API.
type OpenAIProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAI creates a new OpenAI provider.
func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: openaiAPIURL,
		client:   http.DefaultClient,
	}
}

// --- OpenAI Chat Completions API request/response types ---

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

// Chat sends a non-streaming request to the OpenAI Chat Completions API.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []types.Message) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("openai: %w", ErrNotAvailable)
	}

	reqPayload := p.buildRequest(messages)

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: http request: %w", err)
	}
	defer resp.Body.Close()

	if err := CheckResponse("openai", resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: read response: %w", err)
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("openai: parse response JSON: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices in response")
	}

	return apiResp.Choices[0].Message.Content, nil
}

// StreamChat falls back to a non-streaming Chat call.
func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
	resp, err := p.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		ch <- resp
	}()
	return ch, nil
}

func (p *OpenAIProvider) Name() string {
	return "openai/" + p.model
}

func (p *OpenAIProvider) Available() bool {
	return p.apiKey != "" && p.model != ""
}

// SetAvailable toggles availability (for testing fallback behavior).
func (p *OpenAIProvider) SetAvailable(v bool) {
	if !v {
		p.apiKey = ""
	}
}

// buildRequest converts types.Message slice into an openaiRequest.
// System messages use role "system", others map directly.
func (p *OpenAIProvider) buildRequest(messages []types.Message) openaiRequest {
	req := openaiRequest{
		Model:     p.model,
		MaxTokens: openaiMaxTok,
	}

	for _, m := range messages {
		req.Messages = append(req.Messages, openaiMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	return req
}
