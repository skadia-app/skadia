package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// WriteJSONNoCache writes a JSON response with Cache-Control: no-store.
// Use this for all auth endpoints to prevent caching of tokens, credentials,
// and other security-sensitive data.
func WriteJSONNoCache(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, status, v)
}

// WriteError writes an RFC 7807 problem detail response.
func WriteError(w http.ResponseWriter, status int, title, detail, instance string) {
	problem := model.ProblemDetail{
		Type:     "https://skadia.io/errors/" + strings.ReplaceAll(http.StatusText(status), " ", "-"),
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// RequireUserID extracts the user ID from the request context.
// Returns ("", false) and writes 401 if not present.
func RequireUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return "", false
	}
	return userID, true
}

// DecodeJSON decodes the request body into T.
// Returns (zero, false) and writes 400 if decoding fails.
func DecodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return v, false
	}
	return v, true
}

// HandleDBErr writes appropriate HTTP error for database errors.
// Returns true if an error was handled (caller should return).
// Returns false if err is nil (no error).
func HandleDBErr(w http.ResponseWriter, r *http.Request, err error, notFoundMsg string, logAttrs ...any) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pgx.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "Not Found", notFoundMsg, r.URL.Path)
		return true
	}
	slog.Error("database error", append([]any{"error", err, "path", r.URL.Path}, logAttrs...)...)
	WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	return true
}

// MachineError is the machine-readable error response shape for API consumers
// (LLM tool calls, automation clients) that need stable error codes.
type MachineError struct {
	Error   string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteMachineError writes a machine-readable JSON error (not RFC 7807).
// Use for gear/schema validation endpoints where callers process the error code.
func WriteMachineError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(MachineError{
		Error:   code,
		Message: message,
		Details: details,
	}); err != nil {
		slog.Error("failed to encode machine error response", "error", err)
	}
}

// ParsePagination reads limit/offset from query params with defaults and bounds.
func ParsePagination(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = min(parsed, maxLimit)
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}
