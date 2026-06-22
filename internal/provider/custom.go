package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// CustomProvider sends requests to an arbitrary OpenAI-compatible HTTP endpoint.
type CustomProvider struct {
	name           string
	endpoint       string
	apiKey         string
	model          string
	requiresAPIKey bool
	client         *http.Client
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

// SetRequiresAPIKey marks this provider as requiring an API key to be available.
func (p *CustomProvider) SetRequiresAPIKey(v bool) { p.requiresAPIKey = v }

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

// streamRequest is like chatRequest but with stream=true.
type streamRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// streamChunkChoice represents one choice in a streaming chunk.
type streamChunkChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
}

// streamChunk is the SSE payload for OpenAI-compatible streaming.
type streamChunk struct {
	Choices []streamChunkChoice `json:"choices"`
}

func (p *CustomProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
	if !p.Available() {
		return nil, fmt.Errorf("%s: %w", p.name, ErrNotAvailable)
	}

	msgs := make([]chatMessage, len(messages))
	for i, m := range messages {
		msgs[i] = chatMessage{Role: string(m.Role), Content: m.Content}
	}

	reqBody, err := json.Marshal(streamRequest{Model: p.model, Messages: msgs, Stream: true})
	if err != nil {
		return nil, fmt.Errorf("%s: marshal stream request: %w", p.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("%s: create stream request: %w", p.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: stream request: %w", p.name, err)
	}

	if err := CheckResponse(p.name, resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan string, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		parseSSEStream(resp.Body, ch)
	}()
	return ch, nil
}

const sseMaxTokenSize = 512 * 1024 // 512 KiB — handles large SSE events

// parseSSEStream reads OpenAI-compatible SSE lines and sends content deltas
// to ch. Exported for testing.
func parseSSEStream(r io.Reader, ch chan<- string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, sseMaxTokenSize), sseMaxTokenSize)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}
		var chunk streamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- choice.Delta.Content
			}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- fmt.Sprintf("\n[stream error: %v]", err)
	}
}

func (p *CustomProvider) Name() string {
	return p.name
}

func (p *CustomProvider) Available() bool {
	if p.endpoint == "" || p.model == "" {
		return false
	}
	if p.requiresAPIKey && p.apiKey == "" {
		return false
	}
	return true
}

// Verify sends a GET /models request (OpenAI-compatible) to confirm that
// the endpoint is reachable and the API key is valid.
func (p *CustomProvider) Verify(ctx context.Context) error {
	if p.endpoint == "" {
		return fmt.Errorf("%s: %w: no endpoint", p.name, ErrNotAvailable)
	}
	url := strings.TrimSuffix(p.endpoint, "/chat/completions") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%s: create verify request: %w", p.name, err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: verify request: %w", p.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: verify failed: HTTP %d %s", p.name, resp.StatusCode, resp.Status)
	}
	return nil
}
