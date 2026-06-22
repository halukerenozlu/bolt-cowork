package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelsURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{"hosted endpoint", "https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/models"},
		{"local endpoint", "http://localhost:11434/v1/chat/completions", "http://localhost:11434/v1/models"},
		{"trailing slash", "https://api.example.com/v1/chat/completions/", "https://api.example.com/v1/models"},
		{"base URL", "https://api.example.com/v1", "https://api.example.com/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelsURL(tt.endpoint); got != tt.want {
				t.Errorf("modelsURL(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestDiscoverModels(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		status      int
		response    modelsResponse
		responseRaw string
		want        []string
		wantErr     bool
	}{
		{
			name:     "sorted authenticated models",
			apiKey:   "test-key",
			status:   http.StatusOK,
			response: modelsResponse{Data: []modelEntry{{ID: "gpt-4o"}, {ID: "gpt-4o-mini"}, {ID: "gpt-4.1"}}},
			want:     []string{"gpt-4.1", "gpt-4o", "gpt-4o-mini"},
		},
		{
			name:     "local endpoint without key",
			status:   http.StatusOK,
			response: modelsResponse{Data: []modelEntry{{ID: "llama3"}}},
			want:     []string{"llama3"},
		},
		{
			name:    "HTTP error",
			apiKey:  "test-key",
			status:  http.StatusForbidden,
			wantErr: true,
		},
		{
			name:     "empty IDs ignored",
			status:   http.StatusOK,
			response: modelsResponse{Data: []modelEntry{{ID: ""}, {ID: "valid"}}},
			want:     []string{"valid"},
		},
		{
			name:        "invalid JSON",
			status:      http.StatusOK,
			responseRaw: "{invalid",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					http.NotFound(w, r)
					return
				}
				wantAuth := ""
				if tt.apiKey != "" {
					wantAuth = "Bearer " + tt.apiKey
				}
				if got := r.Header.Get("Authorization"); got != wantAuth {
					t.Errorf("Authorization = %q, want %q", got, wantAuth)
				}
				status := tt.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				if tt.responseRaw != "" {
					_, _ = w.Write([]byte(tt.responseRaw))
					return
				}
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			got, err := DiscoverModels(t.Context(), srv.URL+"/v1/chat/completions", tt.apiKey)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DiscoverModels() error = %v, wantErr %v", err, tt.wantErr)
			}
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("DiscoverModels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectLocal(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		response   modelsResponse
		wantFound  bool
		wantModels []string
	}{
		{
			name:       "reachable server with models",
			status:     http.StatusOK,
			response:   modelsResponse{Data: []modelEntry{{ID: "qwen3:8b"}, {ID: "deepseek-r1:8b"}}},
			wantFound:  true,
			wantModels: []string{"deepseek-r1:8b", "qwen3:8b"},
		},
		{
			name:      "server error is not detected",
			status:    http.StatusInternalServerError,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			probes := []localProbe{{
				name:     "test-local",
				endpoint: srv.URL + "/v1/chat/completions",
			}}
			got := detectLocal(context.Background(), srv.Client(), probes)
			if tt.wantFound {
				if len(got) != 1 {
					t.Fatalf("detectLocal() = %v, want one provider", got)
				}
				if strings.Join(got[0].Models, "\x00") != strings.Join(tt.wantModels, "\x00") {
					t.Fatalf("models = %v, want %v", got[0].Models, tt.wantModels)
				}
				return
			}
			if len(got) != 0 {
				t.Fatalf("detectLocal() = %v, want no providers", got)
			}
		})
	}
}
