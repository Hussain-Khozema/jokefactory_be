// Package config handles application configuration via environment variables.
// It uses kelseyhightower/envconfig for parsing and provides sensible defaults.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration.
// Values are loaded from environment variables with the prefix "APP".
// Example: APP_PORT=8080, APP_LOG_LEVEL=debug
type Config struct {
	// Server configuration (embedded to flatten env vars)
	Server ServerConfig

	// Database configuration (embedded to flatten env vars)
	Database DatabaseConfig

	// Logging configuration (embedded to flatten env vars)
	Log LogConfig

	// Admin configuration
	Admin AdminConfig

	// LLM / Azure AI Foundry configuration
	LLM LLMConfig

	// Classification worker pool configuration
	Worker WorkerConfig
}

// WorkerConfig tunes the in-memory classification worker pool.
// One job is one published batch, so the pool width is how many teams get
// classified at the same time — it is the knob that decides whether the last
// team in a class waits seconds or minutes for its feedback.
type WorkerConfig struct {
	// PoolSize is how many batches are classified concurrently.
	//
	// These workers spend nearly all of their time blocked on an Azure LLM
	// call (~15s per joke), so extra workers cost goroutines and almost no CPU:
	// widening the pool shortens the tail, it does not load the host. At 2 a
	// 12-team class drains in ~6 minutes and feedback lands wildly unevenly;
	// at 8 the same class drains in roughly two waves, which is the difference
	// between a usable classroom round and dead air.
	//
	// The real ceiling is the Azure deployment's rate limit, not this host —
	// past the deployment's TPM/RPM quota the extra workers only earn 429s and
	// burn the classifier's retry budget. 8 is chosen to stay comfortably under
	// the quota of a modest gpt-4o-mini deployment; raise it only alongside the
	// Azure quota, and lower it if the classifier starts logging throttling.
	PoolSize int `envconfig:"WORKER_POOL_SIZE" default:"8"`

	// QueueBuffer is how many published batches can wait before Enqueue blocks
	// the publishing request. Sized for a whole class publishing at once.
	QueueBuffer int `envconfig:"WORKER_QUEUE_BUFFER" default:"64"`
}

// LLMConfig holds Azure AI Foundry (OpenAI-compatible) settings.
type LLMConfig struct {
	// BaseURL is the Foundry /openai/v1/ endpoint.
	BaseURL string `envconfig:"LLM_BASE_URL" default:""`

	// APIKey authenticates to Foundry.
	APIKey string `envconfig:"LLM_API_KEY" default:""`

	// Deployment is the model deployment name (e.g. gpt-4o-mini).
	Deployment string `envconfig:"LLM_DEPLOYMENT" default:"gpt-4o-mini"`

	// Temperature for classification (default 0 for determinism).
	Temperature float64 `envconfig:"LLM_TEMPERATURE" default:"0"`

	// MaxRetries is how many times the Azure adapter retries a failed call.
	MaxRetries int `envconfig:"LLM_MAX_RETRIES" default:"3"`
}

// Enabled reports whether LLM credentials are configured.
func (c LLMConfig) Enabled() bool {
	return c.BaseURL != "" && c.APIKey != ""
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Port is the HTTP server port (default: 8080)
	Port int `envconfig:"PORT" default:"8080"`

	// Host is the HTTP server host (default: 0.0.0.0)
	Host string `envconfig:"HOST" default:"0.0.0.0"`

	// ReadTimeout is the maximum duration for reading the entire request (default: 10s)
	ReadTimeout time.Duration `envconfig:"READ_TIMEOUT" default:"10s"`

	// WriteTimeout is the maximum duration before timing out writes of the response (default: 30s)
	WriteTimeout time.Duration `envconfig:"WRITE_TIMEOUT" default:"30s"`

	// ShutdownTimeout is the maximum duration to wait for active connections to finish (default: 30s)
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"30s"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	// Host is the database host (default: localhost)
	Host string `envconfig:"DB_HOST" default:"localhost"`

	// Port is the database port (default: 5432)
	Port int `envconfig:"DB_PORT" default:"5432"`

	// User is the database user (default: postgres)
	User string `envconfig:"DB_USER" default:"postgres"`

	// Password is the database password (required in production)
	Password string `envconfig:"DB_PASSWORD" default:"postgres"`

	// Name is the database name (default: jokefactory)
	Name string `envconfig:"DB_NAME" default:"jokefactory"`

	// SSLMode is the SSL mode for the connection (default: disable)
	SSLMode string `envconfig:"DB_SSLMODE" default:"disable"`

	// MaxOpenConns is the maximum number of open connections (default: 25)
	MaxOpenConns int `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`

	// MaxIdleConns is the maximum number of idle connections (default: 5)
	MaxIdleConns int `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`

	// ConnMaxLifetime is the maximum lifetime of a connection (default: 5m)
	ConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"5m"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	// Level is the log level: debug, info, warn, error (default: info)
	Level string `envconfig:"LOG_LEVEL" default:"info"`

	// Format is the log format: json, text (default: json)
	Format string `envconfig:"LOG_FORMAT" default:"plain"`
}

// AdminConfig holds admin credentials.
type AdminConfig struct {
	// AdminPassword is used for instructor login.
	//
	// Deliberately has no default: a committed default is a password anyone
	// with repo access knows, and a deployment that lost the env var would
	// keep accepting it without saying so. Empty means instructor login is
	// refused outright (see usecase.AdminAuthService.Login), which is the
	// failure we want — visible and closed, not silent and open.
	AdminPassword string `envconfig:"ADMIN_PASSWORD"`
}

// Configured reports whether an admin password was supplied. Callers use this
// to announce a misconfigured deployment at startup instead of discovering it
// when an instructor cannot log in mid-class.
func (c AdminConfig) Configured() bool {
	return c.AdminPassword != ""
}

// DSN returns the PostgreSQL connection string.
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode,
	)
}

// Addr returns the server address in host:port format.
func (c *ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Load reads configuration from environment variables.
// It returns an error if required variables are missing or invalid.
func Load() (*Config, error) {
	var cfg Config

	// Load each config section separately to flatten env var names
	// This allows env vars like APP_PORT instead of APP_SERVER_PORT
	if err := envconfig.Process("APP", &cfg.Server); err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}
	if err := envconfig.Process("APP", &cfg.Database); err != nil {
		return nil, fmt.Errorf("failed to load database config: %w", err)
	}
	if err := envconfig.Process("APP", &cfg.Log); err != nil {
		return nil, fmt.Errorf("failed to load log config: %w", err)
	}
	if err := envconfig.Process("APP", &cfg.Admin); err != nil {
		return nil, fmt.Errorf("failed to load admin config: %w", err)
	}
	if err := envconfig.Process("APP", &cfg.LLM); err != nil {
		return nil, fmt.Errorf("failed to load LLM config: %w", err)
	}
	if err := envconfig.Process("APP", &cfg.Worker); err != nil {
		return nil, fmt.Errorf("failed to load worker config: %w", err)
	}

	return &cfg, nil
}

// MustLoad loads configuration and panics on error.
// Use this only in main.go during startup.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}
