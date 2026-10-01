package httpapi

import (
	"encoding/json"
	"net/http"
)

// apiError is the JSON body of every error response.
type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// Error codes returned in apiError.Code; documented in openapi.yaml.
const (
	codeBadRequest    = "bad_request"
	codeTextTooLong   = "text_too_long"
	codeUnknownVoice  = "unknown_voice"
	codeUnknownLang   = "unknown_lang"
	codeUnauthorized  = "unauthorized"
	codeForbidden     = "forbidden"
	codeNotFound      = "not_found"
	codeConflict      = "conflict"
	codeTooLarge      = "payload_too_large"
	codeRateLimited   = "rate_limited"
	codeBusy          = "busy"
	codeEngineFailure = "engine_failure"
	codeTimeout       = "timeout"
	codeNotReady      = "not_ready"
	codeInternal      = "internal_error"
)

// writeJSON writes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the standard error body.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: msg, Code: code})
}
