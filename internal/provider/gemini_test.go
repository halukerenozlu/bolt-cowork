package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// geminiOKResponse returns a minimal valid Gemini JSON response body.
func geminiOKResponse() string {
	resp := geminiResponse{
		Candidates: []geminiCandidate{
			{Content: geminiContent{Parts: []geminiPart{{Text: "ok"}}}},
		},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

func TestGemini_NoKeyInURL(t *testing.T) {
	testKey := "unit-test-placeholder"
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(geminiOKResponse()))
	}))
	defer srv.Close()

	p := &GeminiProvider{
		apiKey:   testKey,
		model:    "gemini-2.5-flash",
		endpoint: srv.URL,
		client:   srv.Client(),
	}

	_, err := p.Chat(t.Context(), []types.Message{
		{Role: types.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if strings.Contains(gotQuery, testKey) {
		t.Errorf("API key leaked in URL query: %q", gotQuery)
	}
}

func TestGemini_KeyInHeader(t *testing.T) {
	testKey := "unit-test-placeholder"
	var gotHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-goog-api-key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(geminiOKResponse()))
	}))
	defer srv.Close()

	p := &GeminiProvider{
		apiKey:   testKey,
		model:    "gemini-2.5-flash",
		endpoint: srv.URL,
		client:   srv.Client(),
	}

	_, err := p.Chat(t.Context(), []types.Message{
		{Role: types.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if gotHeader != testKey {
		t.Errorf("x-goog-api-key header = %q, want %q", gotHeader, testKey)
	}
}
