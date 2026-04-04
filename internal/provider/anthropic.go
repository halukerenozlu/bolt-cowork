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
	anthropicAPIURL  = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	anthropicMaxTok  = 4096
)

// AnthropicProvider calls the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewAnthropic creates a new Anthropic provider.
func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: anthropicAPIURL,
		client:   http.DefaultClient,
	}
}

// --- Anthropic Messages API request/response types ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
	Type    string             `json:"type"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Chat sends a non-streaming request to the Anthropic Messages API and returns
// the concatenated text blocks from the response.
func (p *AnthropicProvider) Chat(ctx context.Context, messages []types.Message) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("anthropic: %w", ErrNotAvailable)
	}

	reqPayload := p.buildRequest(messages)

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("anthropic: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: http request: %w", err)
	}
	defer resp.Body.Close()

	if err := CheckResponse("anthropic", resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic: read response: %w", err)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("anthropic: parse response JSON: %w", err)
	}

	return extractText(apiResp.Content), nil
}

// StreamChat falls back to a non-streaming Chat call.
func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
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

func (p *AnthropicProvider) Name() string {
	return "anthropic/" + p.model
}

func (p *AnthropicProvider) Available() bool {
	return p.apiKey != "" && p.model != ""
}

// SetAvailable toggles availability (for testing fallback behavior).
func (p *AnthropicProvider) SetAvailable(v bool) {
	if v {
		// Only mark available if credentials exist.
		return
	}
	// Clear apiKey to make Available() return false.
	p.apiKey = ""
}

// buildRequest converts types.Message slice into an anthropicRequest,
// extracting system messages into the top-level system field.
func (p *AnthropicProvider) buildRequest(messages []types.Message) anthropicRequest {
	ar := anthropicRequest{
		Model:     p.model,
		MaxTokens: anthropicMaxTok,
	}

	for _, m := range messages {
		if m.Role == types.RoleSystem {
			if ar.System != "" {
				ar.System += "\n\n"
			}
			ar.System += m.Content
			continue
		}
		ar.Messages = append(ar.Messages, anthropicMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	return ar
}

// extractText concatenates all text blocks from the response content.
func extractText(blocks []anthropicContent) string {
	var result string
	for _, b := range blocks {
		if b.Type == "text" {
			result += b.Text
		}
	}
	return result
}
