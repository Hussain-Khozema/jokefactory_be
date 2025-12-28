// Package main is the entry point for the JokeFactory API server.
// It initializes all dependencies and starts the HTTP server.
package main

import (
	"context"
	"log"
	"os"

	"jokefactory/src/app/server"
	"jokefactory/src/infra/config"
	"jokefactory/src/infra/db"
	"jokefactory/src/infra/logger"
	"jokefactory/src/infra/repo"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Initialize logger
	log := logger.New(cfg.Log)
	log.Info("starting application",
		"port", cfg.Server.Port,
		"log_level", cfg.Log.Level,
	)

	// Initialize database connection
	pg, err := db.New(context.Background(), cfg.Database, log)
	if err != nil {
		return err
	}
	defer pg.Close()

	// Initialize repositories
	gameRepo := repo.NewPostgresRepository(pg, log)

	// Create and run HTTP server
	srv := server.New(cfg, log, gameRepo)

	// Run blocks until shutdown signal is received
	return srv.Run()
}

