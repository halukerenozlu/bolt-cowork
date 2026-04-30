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

// CustomProvider sends requests to an arbitrary OpenAI-compatible HTTP endpoint.
type CustomProvider struct {
	name     string
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

// NewCustom creates a provider that talks to any OpenAI-compatible API.
func NewCustom(name, endpoint, apiKey, model string) *CustomProvider {
	return &CustomProvider{
		name:     name,
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		client:   http.DefaultClient,
	}
}

// chatRequest is the request body for the OpenAI-compatible chat endpoint.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the response body from the OpenAI-compatible chat endpoint.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *CustomProvider) Chat(ctx context.Context, messages []types.Message, _ []ToolSpec) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("%s: %w", p.name, ErrNotAvailable)
	}

	msgs := make([]chatMessage, len(messages))
	for i, m := range messages {
		msgs[i] = chatMessage{Role: string(m.Role), Content: m.Content}
	}

	reqBody, err := json.Marshal(chatRequest{Model: p.model, Messages: msgs})
	if err != nil {
		return "", fmt.Errorf("%s: marshal request: %w", p.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("%s: create request: %w", p.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: http request: %w", p.name, err)
	}
	defer resp.Body.Close()

	if err := CheckResponse(p.name, resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%s: read response: %w", p.name, err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("%s: parse response JSON: %w", p.name, err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("%s: empty choices in response", p.name)
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (p *CustomProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
	// Fallback: non-streaming call piped through a channel.
	resp, err := p.Chat(ctx, messages, nil)
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

func (p *CustomProvider) Name() string {
	return p.name
}

func (p *CustomProvider) Available() bool {
	return p.endpoint != "" && p.model != ""
}
