// Package server provides HTTP server initialization and lifecycle management.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/handler"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/ports"
	"jokefactory/src/infra/config"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	cfg    *config.Config
	log    *slog.Logger
	router *gin.Engine
	http   *http.Server

	// Handlers
	healthHandler *handler.HealthHandler
	sessionHandler *handler.SessionHandler
	roundHandler   *handler.RoundHandler
	batchHandler   *handler.BatchHandler
	qcHandler      *handler.QCHandler
	customerHandler *handler.CustomerHandler
	instructorHandler *handler.InstructorHandler
	adminHandler *handler.AdminHandler
}

// New creates a new Server with all dependencies wired up.
func New(cfg *config.Config, log *slog.Logger, repo ports.GameRepository) *Server {
	// Set Gin mode based on log level
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router without default middleware
	router := gin.New()

	// Create services
	healthService := usecase.NewHealthService(log)
	sessionService := usecase.NewSessionService(repo, log)
	roundService := usecase.NewRoundService(repo, log)
	batchService := usecase.NewBatchService(repo, log)
	qcService := usecase.NewQCService(repo, log)
	customerService := usecase.NewCustomerService(repo, log)
	instructorService := usecase.NewInstructorService(repo, log)
	adminService := usecase.NewAdminAuthService(repo, cfg.Admin.AdminPassword)

	// Create handlers
	healthHandler := handler.NewHealthHandler(healthService)
	sessionHandler := handler.NewSessionHandler(sessionService)
	roundHandler := handler.NewRoundHandler(roundService)
	batchHandler := handler.NewBatchHandler(batchService)
	qcHandler := handler.NewQCHandler(qcService)
	customerHandler := handler.NewCustomerHandler(customerService)
	instructorHandler := handler.NewInstructorHandler(instructorService)
	adminHandler := handler.NewAdminHandler(adminService)

	s := &Server{
		cfg:           cfg,
		log:           log,
		router:        router,
		healthHandler: healthHandler,
		sessionHandler: sessionHandler,
		roundHandler: roundHandler,
		batchHandler: batchHandler,
		qcHandler: qcHandler,
		customerHandler: customerHandler,
		instructorHandler: instructorHandler,
		adminHandler: adminHandler,
	}

	s.setupMiddleware()
	s.setupRoutes()
	s.setupHTTPServer()

	return s
}

// setupMiddleware configures global middleware.
func (s *Server) setupMiddleware() {
	// Order matters: Recovery should be first to catch all panics
	s.router.Use(middleware.Recovery(s.log))
	s.router.Use(middleware.RequestID())
	s.router.Use(middleware.CORS())
	s.router.Use(middleware.Logging(s.log))

	// TODO: Add CORS middleware if needed
	// TODO: Add rate limiting middleware
	// TODO: Add authentication middleware
}

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() {
	// Health check endpoints (no auth required)
	s.router.GET("/health", s.healthHandler.Health)
	s.router.GET("/health/detailed", s.healthHandler.DetailedHealth)

	// API v1 routes
	v1 := s.router.Group("/v1")
	{
		// Session
		v1.POST("/session/join", s.sessionHandler.Join)
		v1.GET("/session/me", s.sessionHandler.Me)

		// Admin/Instructor login
		v1.POST("/instructor/login", s.adminHandler.Login)

		// Rounds
		v1.GET("/rounds/active", s.roundHandler.Active)
		v1.GET("/rounds/:round_id/teams/:team_id/summary", s.roundHandler.TeamSummary)

		// JM batches
		v1.POST("/rounds/:round_id/batches", s.batchHandler.Submit)
		v1.GET("/rounds/:round_id/teams/:team_id/batches", s.batchHandler.List)

		// QC
		v1.GET("/qc/queue/next", s.qcHandler.QueueNext)
		v1.POST("/qc/batches/:batch_id/ratings", s.qcHandler.SubmitRatings)
		v1.GET("/qc/queue/count", s.qcHandler.QueueCount)

		// Customers
		v1.GET("/rounds/:round_id/market", s.customerHandler.Market)
		v1.GET("/rounds/:round_id/customers/budget", s.customerHandler.Budget)
		v1.POST("/rounds/:round_id/market/:joke_id/buy", s.customerHandler.Buy)
		v1.POST("/rounds/:round_id/market/:joke_id/return", s.customerHandler.Return)

		// Instructor
		v1.GET("/instructor/rounds/:round_id/lobby", s.instructorHandler.Lobby)
		v1.POST("/instructor/rounds/:round_id/config", s.instructorHandler.Config)
		v1.POST("/instructor/rounds/:round_id/assign", s.instructorHandler.Assign)
		v1.PATCH("/instructor/rounds/:round_id/users/:user_id", s.instructorHandler.PatchUser)
		v1.POST("/instructor/rounds/:round_id/start", s.instructorHandler.StartRound)
		v1.POST("/instructor/rounds/:round_id/end", s.instructorHandler.EndRound)
		v1.GET("/instructor/rounds/:round_id/stats", s.instructorHandler.Stats)
	}

	// Handle 404
	s.router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":       "NOT_FOUND",
				"message":    "The requested resource was not found",
				"request_id": middleware.GetRequestID(c),
			},
		})
	})
}

// setupHTTPServer configures the underlying HTTP server.
func (s *Server) setupHTTPServer() {
	s.http = &http.Server{
		Addr:         s.cfg.Server.Addr(),
		Handler:      s.router,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
	}
}

// Run starts the HTTP server and blocks until shutdown.
// It handles graceful shutdown on SIGINT/SIGTERM.
func (s *Server) Run() error {
	// Channel to receive shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Channel to receive server errors
	errCh := make(chan error, 1)

	// Start server in goroutine
	go func() {
		s.log.Info("starting HTTP server",
			"addr", s.cfg.Server.Addr(),
		)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-quit:
		s.log.Info("received shutdown signal", "signal", sig.String())
	case err := <-errCh:
		return err
	}

	// Graceful shutdown
	return s.Shutdown()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	s.log.Info("shutting down server", "timeout", s.cfg.Server.ShutdownTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	s.log.Info("server stopped gracefully")
	return nil
}

// Router returns the Gin router for testing.
func (s *Server) Router() *gin.Engine {
	return s.router
}

// WaitForReady waits until the server is ready to accept connections.
// Useful for integration tests.
func (s *Server) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://%s/health", s.cfg.Server.Addr()))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("server not ready after %v", timeout)
}

