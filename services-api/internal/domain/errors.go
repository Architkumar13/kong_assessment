package domain

import "errors"

// Sentinel errors for the domain layer.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when a resource already exists or a unique constraint is violated.
	ErrConflict = errors.New("conflict")

	// ErrValidation is returned when input fails domain validation rules.
	ErrValidation = errors.New("validation error")
)
