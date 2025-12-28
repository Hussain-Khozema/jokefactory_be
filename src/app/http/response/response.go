// Package response defines consistent HTTP response structures.
// All API responses should use these types for consistency.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jokefactory/src/core/domain"
)

// Success represents a successful response with data.
type Success struct {
	Data any `json:"data"`
}

// Error represents an error response.
type Error struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	// Code is a machine-readable error code (e.g., "NOT_FOUND", "VALIDATION_ERROR")
	Code string `json:"code"`

	// Message is a human-readable error description
	Message string `json:"message"`

	// Field is the field that caused the error (for validation errors)
	Field string `json:"field,omitempty"`

	// RequestID is the request ID for debugging
	RequestID string `json:"request_id,omitempty"`
}

// Paginated represents a paginated list response.
type Paginated struct {
	Data       any   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalPages int   `json:"total_pages"`
}

// OK sends a 200 response with data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Success{Data: data})
}

// Created sends a 201 response with the created resource.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Success{Data: data})
}

// NoContent sends a 204 response with no body.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// BadRequest sends a 400 response.
func BadRequest(c *gin.Context, message string, requestID string) {
	c.JSON(http.StatusBadRequest, Error{
		Error: ErrorDetail{
			Code:      "BAD_REQUEST",
			Message:   message,
			RequestID: requestID,
		},
	})
}

// ValidationError sends a 400 response for validation failures.
func ValidationError(c *gin.Context, field, message, requestID string) {
	c.JSON(http.StatusBadRequest, Error{
		Error: ErrorDetail{
			Code:      "VALIDATION_ERROR",
			Message:   message,
			Field:     field,
			RequestID: requestID,
		},
	})
}

// NotFound sends a 404 response.
func NotFound(c *gin.Context, message, requestID string) {
	c.JSON(http.StatusNotFound, Error{
		Error: ErrorDetail{
			Code:      "NOT_FOUND",
			Message:   message,
			RequestID: requestID,
		},
	})
}

// Conflict sends a 409 response.
func Conflict(c *gin.Context, message, requestID string) {
	c.JSON(http.StatusConflict, Error{
		Error: ErrorDetail{
			Code:      "CONFLICT",
			Message:   message,
			RequestID: requestID,
		},
	})
}

// Forbidden sends a 403 response.
func Forbidden(c *gin.Context, message, requestID string) {
	c.JSON(http.StatusForbidden, Error{
		Error: ErrorDetail{
			Code:      "FORBIDDEN",
			Message:   message,
			RequestID: requestID,
		},
	})
}

// Unauthorized sends a 401 response.
func Unauthorized(c *gin.Context, message, requestID string) {
	c.JSON(http.StatusUnauthorized, Error{
		Error: ErrorDetail{
			Code:      "UNAUTHORIZED",
			Message:   message,
			RequestID: requestID,
		},
	})
}

// InternalError sends a 500 response.
func InternalError(c *gin.Context, requestID string) {
	c.JSON(http.StatusInternalServerError, Error{
		Error: ErrorDetail{
			Code:      "INTERNAL_ERROR",
			Message:   "An unexpected error occurred",
			RequestID: requestID,
		},
	})
}

// FromDomainError converts a domain error to an appropriate HTTP response.
// This centralizes error handling and ensures consistent error responses.
func FromDomainError(c *gin.Context, err error, requestID string) {
	switch {
	case domain.IsNotFound(err):
		NotFound(c, err.Error(), requestID)
	case domain.IsValidationError(err):
		// Try to extract field from domain error
		if domainErr, ok := err.(*domain.DomainError); ok {
			ValidationError(c, domainErr.Field, domainErr.Message, requestID)
		} else {
			BadRequest(c, err.Error(), requestID)
		}
	case domain.IsConflict(err):
		Conflict(c, err.Error(), requestID)
	case domain.IsForbidden(err):
		Forbidden(c, err.Error(), requestID)
	case domain.IsUnauthorized(err):
		Unauthorized(c, err.Error(), requestID)
	default:
		InternalError(c, requestID)
	}
}

