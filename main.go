// Package main is the entry point for the JokeFactory API server.
// It initializes all dependencies and starts the HTTP server.
package main

import (
	"context"
	"fmt"
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

	classifier, modelName, err := buildClassifier(cfg.LLM)
	if err != nil {
		return err
	}
	if modelName == "hash" {
		log.Warn("HASH CLASSIFIER ACTIVE: jokes are NOT graded by the LLM; scores are a deterministic hash. Local pilots only, never production.",
			"APP_LLM_MODE", cfg.LLM.Mode,
			"fix", "unset APP_LLM_MODE and APP_LLM_HASH_CONFIRM to grade with the LLM",
		)
	}
	log.Info("classifier selected", "classifier", modelName)
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

// buildClassifier picks the classifier from the LLM settings. Hash mode needs
// both APP_LLM_MODE=hash and APP_LLM_HASH_CONFIRM=local-pilot-only, and an
// unknown mode is an error, so a typo or a single stray variable can never
// silently swap LLM grading for the hash classifier.
func buildClassifier(cfg config.LLMConfig) (c ports.Classifier, model string, err error) {
	switch cfg.Mode {
	case "":
	case "hash":
		if cfg.HashConfirm != config.HashConfirmValue {
			return nil, "", fmt.Errorf(
				"APP_LLM_MODE=hash grades jokes with a hash, not the LLM: set APP_LLM_HASH_CONFIRM=%s to confirm this is a local pilot, or unset APP_LLM_MODE",
				config.HashConfirmValue)
		}
		return llm.HashClassifier{}, "hash", nil
	default:
		return nil, "", fmt.Errorf("APP_LLM_MODE=%q is not supported: leave it unset, or use \"hash\" for local pilots", cfg.Mode)
	}
	if cfg.Enabled() {
		return llm.NewAzureClassifier(cfg), cfg.Deployment, nil
	}
	return llm.StubClassifier{}, "stub", nil
}
