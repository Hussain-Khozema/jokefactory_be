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
	AdminPassword string `envconfig:"ADMIN_PASSWORD" default:"Toyota410"`
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
