package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

var (
	// ErrNoAvailableProvider is returned when all providers in the chain are exhausted.
	ErrNoAvailableProvider = errors.New("no available provider in chain")

	// ErrNotAvailable is returned when a provider is not available.
	ErrNotAvailable = errors.New("provider is not available")
)

// APIError represents an HTTP error returned by an LLM provider API.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
	Provider   string
}

func (e *APIError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s: HTTP %d %s: %s", e.Provider, e.StatusCode, e.Status, e.Body)
	}
	return fmt.Sprintf("%s: HTTP %d %s", e.Provider, e.StatusCode, e.Status)
}

// Retryable reports whether the error is transient and the request may be
// retried on a different provider (429 Too Many Requests, 5xx Server Error).
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// CheckResponse validates an HTTP response from an LLM API. If the status code
// is not 2xx it reads the body (up to 1 KiB), closes the response, and returns
// an *APIError. For retryable errors (429, 5xx) the returned error wraps
// ErrNotAvailable so the fallback chain can switch providers.
func CheckResponse(providerName string, resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
	resp.Body.Close()
	if readErr != nil {
		readErrWrapped := fmt.Errorf("%s: HTTP %d %s (read response body: %w)", providerName, resp.StatusCode, resp.Status, readErr)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return fmt.Errorf("%w: %w", ErrNotAvailable, readErrWrapped)
		}
		return readErrWrapped
	}

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       string(body),
		Provider:   providerName,
	}

	if apiErr.Retryable() {
		return fmt.Errorf("%w: %w", ErrNotAvailable, apiErr)
	}
	return apiErr
}

// LLMProvider abstracts communication with an LLM model.
type LLMProvider interface {
	// Chat sends messages and returns a complete response.
	Chat(ctx context.Context, messages []types.Message) (string, error)

	// StreamChat sends messages and returns a channel of response chunks.
	StreamChat(ctx context.Context, messages []types.Message) (<-chan string, error)

	// Name returns the provider identifier (e.g. "openai/gpt-4o").
	Name() string

	// Available reports whether the provider can accept requests.
	Available() bool
}
