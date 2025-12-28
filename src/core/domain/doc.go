// Package domain contains the core domain model for the application.
//
// This package defines:
//   - Entities: Core business objects with identity (e.g., Joke, User)
//   - Value Objects: Immutable objects defined by their attributes
//   - Domain Errors: Business rule violation errors
//   - Domain Events: Events emitted when domain state changes (optional)
//
// Rules for this package:
//   - No external dependencies except the standard library
//   - No infrastructure concerns (database, HTTP, etc.)
//   - Entities should validate their own invariants
//   - Value objects should be immutable
//
// Example entity structure:
//
//	type Joke struct {
//	    ID        uuid.UUID
//	    Content   string
//	    Category  Category
//	    CreatedAt time.Time
//	    UpdatedAt time.Time
//	}
//
//	func NewJoke(content string, category Category) (*Joke, error) {
//	    if content == "" {
//	        return nil, NewValidationError("content", "cannot be empty")
//	    }
//	    return &Joke{
//	        ID:        uuid.New(),
//	        Content:   content,
//	        Category:  category,
//	        CreatedAt: time.Now(),
//	        UpdatedAt: time.Now(),
//	    }, nil
//	}
//
// TODO: Define domain entities
// TODO: Define value objects
// TODO: Add entity validation methods
package domain

