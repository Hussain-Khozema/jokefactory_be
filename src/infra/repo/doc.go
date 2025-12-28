// Package repo contains PostgreSQL implementations of repository interfaces.
//
// This package implements the ports defined in src/core/ports.
// Each repository is responsible for a specific domain aggregate.
//
// Naming convention:
//   - Files: <entity>_repo.go (e.g., joke_repo.go, user_repo.go)
//   - Types: <Entity>Repository (e.g., JokeRepository, UserRepository)
//
// All repositories receive the database pool via constructor injection
// and implement the corresponding interface from src/core/ports.
//
// Example structure:
//
//	type JokeRepository struct {
//	    db  *db.Postgres
//	    log *slog.Logger
//	}
//
//	func NewJokeRepository(db *db.Postgres, log *slog.Logger) *JokeRepository {
//	    return &JokeRepository{db: db, log: log}
//	}
//
//	func (r *JokeRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Joke, error) {
//	    // TODO: Implement
//	}
//
// TODO: Implement concrete repositories as domain entities are defined
package repo

