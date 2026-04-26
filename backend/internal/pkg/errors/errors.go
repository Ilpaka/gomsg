// Package apperr provides typed application errors for HTTP handlers and services.
// Import path: goflow/backend/internal/pkg/errors
//
// Example (service):
//
//	if user == nil {
//		return apperr.NotFound("user not found")
//	}
//
// Example (handler):
//
//	if err := svc.Do(ctx); err != nil {
//		response.WriteError(w, r, err)
//		return
//	}
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind identifies an error class for clients and HTTP mapping.
type Kind string

const (
	KindUnauthorized     Kind = "unauthorized"
	KindForbidden        Kind = "forbidden"
	KindNotFound         Kind = "not_found"
	KindValidationFailed Kind = "validation_failed"
	KindConflict         Kind = "conflict"
	KindInternal         Kind = "internal"
)

// Error is a typed application error safe to return through transport layers.
// Details is optional structured data (e.g. validation fields); JSON encoding is handled by HTTP layer.
type Error struct {
	Kind    Kind
	Message string
	Details any
	cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

// Unwrap returns the underlying error, if any.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// HTTPStatus maps the error kind to an HTTP status code.
func (e *Error) HTTPStatus() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	switch e.Kind {
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindValidationFailed:
		return http.StatusUnprocessableEntity
	case KindConflict:
		return http.StatusConflict
	case KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// New constructs an Error with optional wrapped cause.
func New(kind Kind, message string, cause error) *Error {
	return &Error{Kind: kind, Message: message, cause: cause}
}

// WithDetails returns a shallow copy of err with Details set (for *Error only).
func WithDetails(err error, details any) error {
	if ae, ok := As(err); ok {
		cp := *ae
		cp.Details = details
		return &cp
	}
	return err
}

// Unauthorized is a 401-class error (e.g. bad credentials or missing token).
func Unauthorized(message string) *Error {
	return New(KindUnauthorized, message, nil)
}

// Forbidden is a 403-class error.
func Forbidden(message string) *Error {
	return New(KindForbidden, message, nil)
}

// NotFound is a 404-class error.
func NotFound(message string) *Error {
	return New(KindNotFound, message, nil)
}

// Validation wraps validation failures (optionally with a cause).
func Validation(message string, cause error) *Error {
	return New(KindValidationFailed, message, cause)
}

// ValidationDetails is a validation error with structured details (e.g. field list).
func ValidationDetails(message string, details any) *Error {
	return &Error{Kind: KindValidationFailed, Message: message, Details: details}
}

// Conflict indicates a domain conflict (e.g. duplicate email).
func Conflict(message string) *Error {
	return New(KindConflict, message, nil)
}

// Internal wraps an unexpected error for logging while returning a safe message.
func Internal(message string, cause error) *Error {
	return New(KindInternal, message, cause)
}

// As returns (*Error, true) if err unwraps to *Error.
func As(err error) (*Error, bool) {
	var ae *Error
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// HTTPStatus returns the status for err, defaulting to 500 if not an *Error.
func HTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if ae, ok := As(err); ok {
		return ae.HTTPStatus()
	}
	return http.StatusInternalServerError
}
