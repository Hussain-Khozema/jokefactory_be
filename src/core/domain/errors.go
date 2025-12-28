// Package domain contains domain entities, value objects, and domain-specific errors.
// This package should have no external dependencies except the standard library.
package domain

import (
	"errors"
	"fmt"
)

// Domain error types for consistent error handling across the application.
// These errors represent business rule violations and domain constraints.

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists is returned when trying to create a resource that already exists.
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized is returned when authentication is required but not provided.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when the user lacks permission for the operation.
	ErrForbidden = errors.New("forbidden")

	// ErrConflict is returned when there's a conflict with the current state.
	ErrConflict = errors.New("conflict")
)

// DomainError wraps a base error with additional context.
// It provides a standard way to add details to domain errors.
type DomainError struct {
	// Base is the underlying error type (e.g., ErrNotFound)
	Base error

	// Message provides human-readable context
	Message string

	// Field indicates which field caused the error (for validation errors)
	Field string
}

// Error implements the error interface.
func (e *DomainError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field: %s)", e.Base.Error(), e.Message, e.Field)
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Base.Error(), e.Message)
	}
	return e.Base.Error()
}

// Unwrap returns the base error for errors.Is/As support.
func (e *DomainError) Unwrap() error {
	return e.Base
}

// NewNotFoundError creates a not found error with context.
func NewNotFoundError(resource string) *DomainError {
	return &DomainError{
		Base:    ErrNotFound,
		Message: resource,
	}
}

// NewValidationError creates a validation error for a specific field.
func NewValidationError(field, message string) *DomainError {
	return &DomainError{
		Base:    ErrInvalidInput,
		Message: message,
		Field:   field,
	}
}

// NewConflictError creates a conflict error with context.
func NewConflictError(message string) *DomainError {
	return &DomainError{
		Base:    ErrConflict,
		Message: message,
	}
}

// NewForbiddenError creates a forbidden error with context.
func NewForbiddenError(message string) *DomainError {
	return &DomainError{
		Base:    ErrForbidden,
		Message: message,
	}
}

// NewUnauthorizedError creates an unauthorized error with context.
func NewUnauthorizedError(message string) *DomainError {
	return &DomainError{
		Base:    ErrUnauthorized,
		Message: message,
	}
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// IsConflict checks if an error is a conflict error.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// IsForbidden checks if an error is a forbidden error.
func IsForbidden(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsUnauthorized checks if an error is unauthorized.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// TODO: Add domain entities (e.g., Joke, User, Category)
// TODO: Add value objects (e.g., JokeID, UserID)
// TODO: Add domain events if using event-driven patterns

