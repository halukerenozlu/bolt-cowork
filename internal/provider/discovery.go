package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// modelsResponse is the OpenAI-compatible /models response envelope.
type modelsResponse struct {
	Data []modelEntry `json:"data"`
}

type modelEntry struct {
	ID string `json:"id"`
}

// DiscoverModels fetches the model list from an OpenAI-compatible /models
// endpoint. Returns model IDs sorted alphabetically. The caller should supply
// the base URL (e.g. "https://api.openai.com/v1/chat/completions"); the
// function strips "/chat/completions" and appends "/models".
func DiscoverModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	return discoverModels(ctx, http.DefaultClient, endpoint, apiKey)
}

func discoverModels(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]string, error) {
	url := modelsURL(endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("discover models: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover models: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discover models: HTTP %d %s", resp.StatusCode, resp.Status)
	}

	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("discover models: parse response: %w", err)
	}

	ids := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// modelsURL converts a chat completions endpoint to its /models sibling.
func modelsURL(endpoint string) string {
	base := strings.TrimRight(endpoint, "/")
	base = strings.TrimSuffix(base, "/chat/completions")
	return base + "/models"
}

// LocalProvider describes a detected local inference server.
type LocalProvider struct {
	Name     string   // e.g. "ollama", "lmstudio"
	Endpoint string   // e.g. "http://localhost:11434/v1/chat/completions"
	Models   []string // installed model IDs (empty if detection succeeded but model list failed)
}

// localProbe defines a local server to check.
type localProbe struct {
	name     string
	endpoint string
}

var localProbes = []localProbe{
	{"ollama", "http://localhost:11434/v1/chat/completions"},
	{"lmstudio", "http://localhost:1234/v1/chat/completions"},
}

// DetectLocal probes well-known local inference servers and returns those that
// are reachable. Each probe has a 2-second timeout bounded by ctx.
func DetectLocal(ctx context.Context) []LocalProvider {
	return detectLocal(ctx, http.DefaultClient, localProbes)
}

func detectLocal(ctx context.Context, client *http.Client, probes []localProbe) []LocalProvider {
	type result struct {
		provider LocalProvider
		ok       bool
	}

	ch := make(chan result, len(probes))
	for _, probe := range probes {
		go func(p localProbe) {
			probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			url := modelsURL(p.endpoint)
			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
			if err != nil {
				ch <- result{}
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				ch <- result{}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				ch <- result{}
				return
			}

			var mr modelsResponse
			var models []string
			if json.NewDecoder(resp.Body).Decode(&mr) == nil {
				for _, m := range mr.Data {
					if m.ID != "" {
						models = append(models, m.ID)
					}
				}
				sort.Strings(models)
			}

			ch <- result{
				provider: LocalProvider{Name: p.name, Endpoint: p.endpoint, Models: models},
				ok:       true,
			}
		}(probe)
	}

	var detected []LocalProvider
	for range probes {
		if r := <-ch; r.ok {
			detected = append(detected, r.provider)
		}
	}
	return detected
}
