package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"

	apperr "github.com/itsChris/wgpilot/internal/errors"
	"github.com/itsChris/wgpilot/internal/logging"
)

// errorResponse is the JSON shape returned for all API errors.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Stack     string `json:"stack,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a structured JSON error response.
// In dev mode, the full error chain, an abbreviated stack trace, and an
// actionable hint are included.
func writeError(w http.ResponseWriter, r *http.Request, err error, code string, status int, devMode bool) {
	requestID := logging.RequestID(r.Context())

	body := errorBody{
		Code:      code,
		Message:   sanitizeError(err),
		RequestID: requestID,
	}

	if devMode && err != nil {
		body.Detail = err.Error()
		body.Stack = captureStack(3)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: body})
}

// sanitizeError returns a user-safe error message. It strips internal
// implementation details that should not leak in production.
func sanitizeError(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

// decodeJSON reads and decodes a JSON request body into v.
// It detects MaxBytesError (body exceeded the configured limit) and returns
// the appropriate HTTP status code (413 vs 400).
func decodeJSON(r *http.Request, v any) (code string, status int, err error) {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apperr.ErrValidation, http.StatusRequestEntityTooLarge,
				fmt.Errorf("request body too large (limit %d bytes)", maxBytesErr.Limit)
		}
		return apperr.ErrValidation, http.StatusBadRequest,
			fmt.Errorf("invalid request body: %w", err)
	}
	return "", 0, nil
}

// fieldError describes a single field-level validation error.
type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// validationErrorResponse is the JSON shape for 400 validation errors.
type validationErrorResponse struct {
	Error     string       `json:"error"`
	Code      string       `json:"code"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []fieldError `json:"fields"`
}

// writeValidationError writes a structured validation error with field-level details.
func writeValidationError(w http.ResponseWriter, r *http.Request, fields []fieldError) {
	requestID := logging.RequestID(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(validationErrorResponse{
		Error:     "validation failed",
		Code:      apperr.ErrValidation,
		RequestID: requestID,
		Fields:    fields,
	})
}

// captureStack returns an abbreviated stack trace starting skip frames up.
func captureStack(skip int) string {
	pcs := make([]uintptr, 5)
	n := runtime.Callers(skip+1, pcs)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs[:n])
	var stack string
	for {
		frame, more := frames.Next()
		if stack != "" {
			stack += " -> "
		}
		stack += fmt.Sprintf("%s:%d", frame.File, frame.Line)
		if !more {
			break
		}
	}
	return stack
}
