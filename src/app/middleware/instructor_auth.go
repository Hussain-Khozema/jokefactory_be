package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/response"
	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

const instructorHeader = "X-User-Id"

// InstructorAuth enforces that the incoming request is made by an instructor.
// It reads the X-User-Id header, validates the user exists, and checks the role.
// On success it stores the user ID in the context under the key "user_id".
func InstructorAuth(repo ports.GameRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := GetRequestID(c)

		userIDStr := c.GetHeader(instructorHeader)
		if userIDStr == "" {
			response.Unauthorized(c, "missing X-User-Id header", requestID)
			c.Abort()
			return
		}

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil || userID <= 0 {
			response.BadRequest(c, "invalid X-User-Id", requestID)
			c.Abort()
			return
		}

		user, err := repo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			response.Unauthorized(c, "user not found", requestID)
			c.Abort()
			return
		}

		if user.Role == nil || *user.Role != domain.RoleInstructor {
			response.Forbidden(c, "user must be an instructor", requestID)
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}
