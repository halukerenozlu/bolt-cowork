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

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// GeminiProvider calls the Google Gemini API.
type GeminiProvider struct {
	apiKey   string
	model    string
	endpoint string // base URL, default: geminiBaseURL
	client   *http.Client
}

// NewGemini creates a new Gemini provider.
func NewGemini(apiKey, model string) *GeminiProvider {
	return &GeminiProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: geminiBaseURL,
		client:   http.DefaultClient,
	}
}

// --- Gemini API request/response types ---

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

// Chat sends a non-streaming request to the Gemini generateContent API.
func (p *GeminiProvider) Chat(ctx context.Context, messages []types.Message, _ []ToolSpec) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("gemini: %w", ErrNotAvailable)
	}

	reqPayload := p.buildRequest(messages)

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", p.endpoint, p.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("gemini: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini: http request: %w", err)
	}
	defer resp.Body.Close()

	if err := CheckResponse("gemini", resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gemini: read response: %w", err)
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("gemini: parse response JSON: %w", err)
	}

	if len(apiResp.Candidates) == 0 {
		return "", fmt.Errorf("gemini: empty candidates in response")
	}

	return extractGeminiText(apiResp.Candidates[0].Content.Parts), nil
}

// StreamChat falls back to a non-streaming Chat call.
func (p *GeminiProvider) StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error) {
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

func (p *GeminiProvider) Name() string {
	return "gemini/" + p.model
}

func (p *GeminiProvider) Available() bool {
	return p.apiKey != "" && p.model != ""
}

// Verify sends a GET /v1beta/models request to confirm the API key is valid.
func (p *GeminiProvider) Verify(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("gemini: %w: no API key", ErrNotAvailable)
	}
	url := p.endpoint + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("gemini: create verify request: %w", err)
	}
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("gemini: verify request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gemini: verify failed: HTTP %d %s", resp.StatusCode, resp.Status)
	}
	return nil
}

// buildRequest converts types.Message slice into a geminiRequest.
// System messages go into systemInstruction, user→"user", assistant→"model".
func (p *GeminiProvider) buildRequest(messages []types.Message) geminiRequest {
	var req geminiRequest
	var systemParts []geminiPart

	for _, m := range messages {
		switch m.Role {
		case types.RoleSystem:
			systemParts = append(systemParts, geminiPart{Text: m.Content})
		case types.RoleUser:
			req.Contents = append(req.Contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: m.Content}},
			})
		case types.RoleAssistant:
			req.Contents = append(req.Contents, geminiContent{
				Role:  "model",
				Parts: []geminiPart{{Text: m.Content}},
			})
		}
	}

	if len(systemParts) > 0 {
		req.SystemInstruction = &geminiContent{Parts: systemParts}
	}

	return req
}

// extractGeminiText concatenates all text parts from the response.
func extractGeminiText(parts []geminiPart) string {
	var result string
	for _, p := range parts {
		result += p.Text
	}
	return result
}
