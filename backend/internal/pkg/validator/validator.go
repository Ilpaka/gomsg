package validator

import "strings"

// Violation is a single field-level validation problem for API error details.
type Violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// FieldViolations wraps a list of violations for response.details.
type FieldViolations struct {
	Fields []Violation `json:"fields"`
}

// Details builds a details object suitable for apperr.ValidationDetails / response.WriteError.
func Details(violations []Violation) any {
	if len(violations) == 0 {
		return map[string]any{"fields": []Violation{}}
	}
	return FieldViolations{Fields: violations}
}

// Add appends a violation when cond is true (invalid).
func Add(v []Violation, field, message string, cond bool) []Violation {
	if !cond {
		return v
	}
	return append(v, Violation{Field: field, Message: message})
}

// NonEmpty adds a violation when trimmed value is empty.
func NonEmpty(v []Violation, field, value string) []Violation {
	if strings.TrimSpace(value) == "" {
		return append(v, Violation{Field: field, Message: "is required"})
	}
	return v
}
