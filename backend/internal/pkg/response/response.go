package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	apperr "goflow/backend/internal/pkg/errors"
)

// SuccessBody is a minimal success envelope; use Data for payloads.
type SuccessBody struct {
	OK   bool `json:"ok"`
	Data any  `json:"data,omitempty"`
}

// WriteJSON writes JSON with the given status and payload (no envelope).
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteSuccess writes { "ok": true, "data": data } with HTTP 200 unless status overrides.
func WriteSuccess(w http.ResponseWriter, status int, data any) {
	if status == 0 {
		status = http.StatusOK
	}
	WriteJSON(w, status, SuccessBody{OK: true, Data: data})
}

func normalizeDetails(d any) any {
	if d == nil {
		return map[string]any{}
	}
	return d
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, details any) {
	payload := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": normalizeDetails(details),
		},
	}
	WriteJSON(w, status, payload)
}

// WriteError maps err to unified JSON and HTTP status.
//
// Format:
//
//	{ "error": { "code": "...", "message": "...", "details": {} } }
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	if err == nil {
		WriteJSON(w, http.StatusOK, SuccessBody{OK: true})
		return
	}

	status := apperr.HTTPStatus(err)
	code := "internal"
	message := "internal server error"
	var details any

	if ae, ok := apperr.As(err); ok {
		code = string(ae.Kind)
		message = ae.Message
		details = ae.Details
	} else {
		if log != nil {
			log.Error("unhandled error", "err", err, "path", r.URL.Path)
		}
	}

	writeAPIError(w, status, code, message, details)
}

// WriteErrorWithDetails is like WriteError but merges explicit details when the error has none.
func WriteErrorWithDetails(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error, details any) {
	if err == nil {
		WriteSuccess(w, http.StatusOK, nil)
		return
	}
	status := apperr.HTTPStatus(err)
	code := "internal"
	message := "internal server error"
	var merged any

	if ae, ok := apperr.As(err); ok {
		code = string(ae.Kind)
		message = ae.Message
		if ae.Details != nil {
			merged = ae.Details
		} else {
			merged = details
		}
	} else {
		if log != nil {
			log.Error("unhandled error", "err", err, "path", r.URL.Path)
		}
		merged = details
	}

	writeAPIError(w, status, code, message, merged)
}

// Is reports whether err unwraps to an apperr.Error of the given kind.
func Is(err error, kind apperr.Kind) bool {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Kind == kind
}
