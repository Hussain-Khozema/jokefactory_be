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
	)

	pg, err := db.New(context.Background(), cfg.Database, log)
	if err != nil {
		return err
	}
	defer pg.Close()

	gameRepo := postgres.New(pg, log)

	classifier, modelName := buildClassifier(cfg)
	aiCustomers := usecase.NewAICustomerService(gameRepo, nil, log)
	classSvc := usecase.NewClassificationService(gameRepo, classifier, aiCustomers, modelName, log)
	dispatcher := worker.NewDispatcher(classSvc, worker.DefaultDispatcherConfig(), log)
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

func buildClassifier(cfg *config.Config) (c ports.Classifier, model string) {
	if cfg.LLM.Enabled() {
		return llm.NewAzureClassifier(cfg.LLM), cfg.LLM.Deployment
	}
	return llm.StubClassifier{}, "stub"
}
