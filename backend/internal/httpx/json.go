package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// WriteJSON writes v as a JSON body with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}

// WriteError writes a JSON {"error": msg} body with the given status.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]any{"error": msg})
}

// PathID parses a numeric path value set by a ServeMux wildcard, e.g. the
// "{id}" in "GET /api/decks/{id}". The second return is false when the
// segment is absent or not a number, which callers should treat as a 404
// rather than a 400: a non-numeric ID names a resource that cannot exist.
func PathID(r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
