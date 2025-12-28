// Package dto contains Data Transfer Objects for HTTP requests and responses.
//
// DTOs are separate from domain entities to:
//   - Control what data is exposed in the API
//   - Handle JSON serialization/deserialization
//   - Add validation tags for request binding
//   - Version the API without changing domain models
//
// Naming convention:
//   - Request types: <Action><Resource>Request (e.g., CreateJokeRequest)
//   - Response types: <Resource>Response (e.g., JokeResponse)
//
// Example:
//
//	type CreateJokeRequest struct {
//	    Content  string `json:"content" binding:"required,min=1,max=1000"`
//	    Category string `json:"category" binding:"required"`
//	}
//
//	func (r *CreateJokeRequest) ToInput() usecase.CreateJokeInput {
//	    return usecase.CreateJokeInput{
//	        Content:  r.Content,
//	        Category: r.Category,
//	    }
//	}
//
//	type JokeResponse struct {
//	    ID        string    `json:"id"`
//	    Content   string    `json:"content"`
//	    Category  string    `json:"category"`
//	    CreatedAt time.Time `json:"created_at"`
//	}
//
//	func (JokeResponse) FromDomain(j *domain.Joke) JokeResponse {
//	    return JokeResponse{
//	        ID:        j.ID.String(),
//	        Content:   j.Content,
//	        Category:  string(j.Category),
//	        CreatedAt: j.CreatedAt,
//	    }
//	}
//
// TODO: Add DTO definitions for business endpoints
package dto

