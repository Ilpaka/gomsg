package metrics

import (
	apperr "goflow/backend/internal/pkg/errors"
)

// AuthFailureFromErr maps application errors to low-cardinality auth failure labels.
func AuthFailureFromErr(err error) string {
	ae, ok := apperr.As(err)
	if !ok {
		return "other"
	}
	switch ae.Kind {
	case apperr.KindUnauthorized:
		return "unauthorized"
	case apperr.KindForbidden:
		return "forbidden"
	case apperr.KindNotFound:
		return "not_found"
	case apperr.KindValidationFailed:
		return "validation"
	case apperr.KindConflict:
		return "conflict"
	case apperr.KindInternal:
		return "internal"
	default:
		return "other"
	}
}
