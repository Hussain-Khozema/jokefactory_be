package middleware

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logging(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Capture request body
		var reqBodyBytes []byte
		if c.Request.Body != nil {
			reqBodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))
		}

		// Capture response body
		rec := &responseCapture{ResponseWriter: c.Writer}
		c.Writer = rec

		// Process request
		c.Next()

		requestID := GetRequestID(c)
		api := path
		if query != "" {
			api = api + "?" + query
		}

		reqBody := string(reqBodyBytes)
		respBody := rec.body.String()

		logLine := fmt.Sprintf("%s | %s | %s | %s | request: %s | response: %s |",
			time.Now().Format(time.RFC3339Nano),
			levelString(c.Writer.Status()),
			requestID,
			api,
			reqBody,
			respBody,
		)

		// Choose log level based on status code and emit single-line log
		status := c.Writer.Status()
		switch {
		case status >= 500:
			log.Error(logLine)
		case status >= 400:
			log.Warn(logLine)
		default:
			log.Info(logLine)
		}
	}
}

// responseCapture captures response body while delegating to original writer.
type responseCapture struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteString(s string) (int, error) {
	r.body.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}

func levelString(status int) string {
	switch {
	case status >= 500:
		return "ERROR"
	case status >= 400:
		return "WARN"
	default:
		return "INFO"
	}
}

