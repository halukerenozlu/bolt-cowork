package handler

import "net/http"

// Handler handles HTTP requests.
type Handler struct {
	version string
}

// New returns a Handler with the given version tag.
func New(version string) *Handler {
	return &Handler{version: version}
}

// ServeHTTP handles incoming HTTP requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Version", h.version)
	w.WriteHeader(http.StatusOK)
}
