// Package main is the entry point for the JokeFactory API server.
// It initializes all dependencies and starts the HTTP server.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"jokefactory/src/app/server"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
	"jokefactory/src/infra/config"
	"jokefactory/src/infra/db"
	"jokefactory/src/infra/llm"
	"jokefactory/src/infra/logger"
	"jokefactory/src/infra/repo/postgres"
	"jokefactory/src/infra/worker"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log)
	log.Info("starting application",
		"port", cfg.Server.Port,
		"log_level", cfg.Log.Level,
		"classification_workers", cfg.Worker.PoolSize,
	)

	// Students never need the admin password, so a missing one must not take
	// the whole class offline — but it must not pass unnoticed either. Instructor
	// login is already refused when it is empty; this is the part that says so
	// out loud, at boot, instead of leaving it to be discovered mid-class.
	if !cfg.Admin.Configured() {
		log.Warn("admin password is not configured: instructor login will be refused",
			"fix", "set APP_ADMIN_PASSWORD",
		)
	}

	pg, err := db.New(context.Background(), cfg.Database, log)
	if err != nil {
		return err
	}
	defer pg.Close()

	gameRepo := postgres.New(pg, log)

	classifier, modelName := buildClassifier(cfg)
	aiCustomers := usecase.NewAICustomerService(gameRepo, nil, log)
	classSvc := usecase.NewClassificationService(gameRepo, classifier, aiCustomers, modelName, log)
	dispatcher := worker.NewDispatcher(classSvc, dispatcherConfig(cfg), log)
	reconciler := worker.NewReconciler(gameRepo, dispatcher, time.Minute, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatcher.Start(ctx)
	reconciler.Start(ctx)
	defer dispatcher.Stop()
	defer reconciler.Stop()

	srv := server.New(cfg, log, gameRepo, dispatcher, aiCustomers)
	return srv.Run()
}

// dispatcherConfig translates env-driven worker settings into the pool's own
// config. NewDispatcher still guards non-positive values, so a bad env var
// degrades to the package default rather than to a pool that never runs.
func dispatcherConfig(cfg *config.Config) worker.DispatcherConfig {
	return worker.DispatcherConfig{
		Workers: cfg.Worker.PoolSize,
		Buffer:  cfg.Worker.QueueBuffer,
	}
}

func buildClassifier(cfg *config.Config) (c ports.Classifier, model string) {
	if cfg.LLM.Enabled() {
		return llm.NewAzureClassifier(cfg.LLM), cfg.LLM.Deployment
	}
	return llm.StubClassifier{}, "stub"
}
