# JokeFactory API

A Go HTTP API server built with Gin, following clean architecture principles.

## Architecture Overview

This project follows a **clean architecture-inspired** layered structure that separates concerns and enforces dependency rules:

```
┌─────────────────────────────────────────────────────────────┐
│                        HTTP Clients                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     src/app (HTTP Layer)                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Server    │  │  Handlers   │  │     Middleware      │  │
│  │  (router)   │  │   (DTOs)    │  │  (request-id, log)  │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   src/core (Business Logic)                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Domain    │  │    Ports    │  │      Use Cases      │  │
│  │ (entities)  │  │(interfaces) │  │    (services)       │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                 src/infra (Infrastructure)                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Config    │  │   Logger    │  │     Database        │  │
│  │   (env)     │  │   (slog)    │  │    (postgres)       │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              Repositories (port implementations)         ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### Dependency Rules

- **src/app** → depends on → **src/core** and **src/infra**
- **src/core** → depends on → nothing (pure Go, no external deps)
- **src/infra** → depends on → **src/core** (implements ports)

## Folder Structure

```
.
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── Makefile                # Build and dev tasks
├── Dockerfile              # Multi-stage Docker build
├── docker-compose.yml      # Local development stack
├── .golangci.yml           # Linter configuration
├── .editorconfig           # Editor settings
└── src/
    ├── app/                # HTTP/Application layer
    │   ├── server/         # HTTP server bootstrap, router setup
    │   ├── http/           # HTTP handlers and DTOs
    │   │   ├── handler/    # Request handlers
    │   │   ├── dto/        # Request/Response objects
    │   │   └── response/   # Response helpers
    │   └── middleware/     # HTTP middleware
    │
    ├── core/               # Business/Domain layer
    │   ├── domain/         # Entities, value objects, domain errors
    │   ├── ports/          # Interfaces for repositories/services
    │   └── usecase/        # Business logic orchestration
    │
    └── infra/              # Infrastructure layer
        ├── config/         # Environment configuration
        ├── logger/         # Logging setup (slog)
        ├── db/             # Database connection
        └── repo/           # Repository implementations
```

## Getting Started

### Prerequisites

- Go 1.23+
- Docker and Docker Compose (optional, for containerized development)
- Make (optional, for convenience)

### Run Locally

```bash
# Install dependencies
go mod download

# Run the server
go run .

# Or use Make
make run
```

The server starts on `http://localhost:8080` by default.

### Run with Docker

```bash
# Build and start all services
make docker-up

# View logs
make docker-logs

# Stop services
make docker-down
```

### Verify It Works

```bash
# Health check
curl http://localhost:8080/health
# {"status":"ok"}

# Detailed health (includes component status)
curl http://localhost:8080/health/detailed
```

## Configuration

Configuration is loaded from environment variables with the `APP_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_PORT` | `8080` | HTTP server port |
| `APP_HOST` | `0.0.0.0` | HTTP server host |
| `APP_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `APP_LOG_FORMAT` | `json` | Log format (json, text) |
| `APP_READ_TIMEOUT` | `10s` | HTTP read timeout |
| `APP_WRITE_TIMEOUT` | `30s` | HTTP write timeout |
| `APP_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `APP_DB_HOST` | `localhost` | PostgreSQL host |
| `APP_DB_PORT` | `5432` | PostgreSQL port |
| `APP_DB_USER` | `postgres` | PostgreSQL user |
| `APP_DB_PASSWORD` | `postgres` | PostgreSQL password |
| `APP_DB_NAME` | `jokefactory` | PostgreSQL database |
| `APP_DB_SSLMODE` | `disable` | PostgreSQL SSL mode |

## Development

### Available Make Targets

```bash
make help         # Show all targets
make run          # Run the application
make build        # Build binary
make test         # Run tests
make lint         # Run linter
make fmt          # Format code
make tidy         # Tidy go.mod
make verify       # Run all checks (fmt, lint, test)
make clean        # Remove build artifacts
make docker-up    # Start Docker services
make docker-down  # Stop Docker services
```

### Adding a New Endpoint

1. **Define domain entity** in `src/core/domain/`
2. **Define repository interface** in `src/core/ports/`
3. **Implement repository** in `src/infra/repo/`
4. **Create use case** in `src/core/usecase/`
5. **Add DTOs** in `src/app/http/dto/`
6. **Create handler** in `src/app/http/handler/`
7. **Register route** in `src/app/server/server.go`
8. **Wire dependencies** in `main.go`

### Example: Adding a Jokes Endpoint

```go
// 1. src/core/domain/joke.go
type Joke struct {
    ID      uuid.UUID
    Content string
}

// 2. src/core/ports/repositories.go
type JokeRepository interface {
    Create(ctx context.Context, joke *domain.Joke) error
    FindByID(ctx context.Context, id uuid.UUID) (*domain.Joke, error)
}

// 3. src/infra/repo/joke_repo.go
type JokeRepository struct { ... }
func (r *JokeRepository) Create(...) error { ... }

// 4. src/core/usecase/joke_service.go
type JokeService struct { jokeRepo ports.JokeRepository }
func (s *JokeService) CreateJoke(...) (*domain.Joke, error) { ... }

// 5-6. Add handler and DTOs

// 7. src/app/server/server.go - register routes
jokes := v1.Group("/jokes")
jokes.POST("", s.jokeHandler.Create)

// 8. main.go - wire dependencies
jokeRepo := repo.NewJokeRepository(db, log)
jokeService := usecase.NewJokeService(jokeRepo, log)
```

## Testing

```bash
# Run all tests
make test

# Run with coverage
make coverage

# Run short tests only (skip integration)
make test-short
```

## Why These Choices?

### slog over zap
- Part of Go standard library (1.21+)
- Zero external dependencies
- Idiomatic for new Go projects
- Comparable performance for most use cases

### Gin as HTTP router
- Battle-tested, widely adopted
- Good performance
- Rich middleware ecosystem
- Excellent documentation

### pgx over database/sql
- Native PostgreSQL driver (no CGO)
- Better performance
- First-class support for PostgreSQL types
- Connection pooling built-in

## License

See [LICENSE](LICENSE) file.
