package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// QCService handles quality control flows.
type QCService struct {
	repo ports.GameRepository
	log  *slog.Logger
}

func NewQCService(repo ports.GameRepository, log *slog.Logger) *QCService {
	return &QCService{repo: repo, log: log}
}

type QCQueueItem struct {
	Batch      domain.Batch
	Jokes      []domain.Joke
	QueueSize  int
}

func (s *QCService) Next(ctx context.Context, userID, roundID int64) (*QCQueueItem, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Role == nil || *user.Role != domain.RoleQC {
		return nil, domain.NewForbiddenError("user must be QC")
	}
	if user.TeamID == nil {
		return nil, domain.NewConflictError("qc user missing team assignment")
	}

	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, domain.NewConflictError("round not active")
	}

	bw, size, err := s.repo.GetNextBatchForQC(ctx, roundID, userID, *user.TeamID)
	if err != nil {
		return nil, err
	}
	return &QCQueueItem{Batch: bw.Batch, Jokes: bw.Jokes, QueueSize: size}, nil
}

func (s *QCService) Rate(ctx context.Context, userID, batchID int64, ratings []domain.JokeRating, feedback *string) (*domain.Batch, []int64, error) {
	if len(ratings) == 0 {
		return nil, nil, domain.NewValidationError("ratings", "at least one rating required")
	}
	// Validate tags and feedback requirement for OTHER.
	requiresFeedback := false
	for _, r := range ratings {
		switch r.Tag {
		case domain.QCTagExcellentStandout, domain.QCTagGenuinelyFunny, domain.QCTagMadeMeSmile,
			domain.QCTagOriginalIdea, domain.QCTagPoliteSmile, domain.QCTagDidntLand,
			domain.QCTagNotAcceptable, domain.QCTagOther:
		default:
			return nil, nil, domain.NewValidationError("tag", "invalid tag value")
		}
		if r.Tag == domain.QCTagOther {
			requiresFeedback = true
		}
	}
	if requiresFeedback {
		if feedback == nil || len(*feedback) == 0 {
			return nil, nil, domain.NewValidationError("feedback", "feedback required when tag is OTHER")
		}
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user.Role == nil || *user.Role != domain.RoleQC {
		return nil, nil, domain.NewForbiddenError("user must be QC")
	}

	bw, err := s.repo.GetBatchWithJokes(ctx, batchID)
	if err != nil {
		return nil, nil, err
	}
	round, err := s.repo.GetRoundByID(ctx, bw.Batch.RoundID)
	if err != nil {
		return nil, nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, nil, domain.NewConflictError("round not active")
	}
	if len(ratings) != len(bw.Jokes) {
		return nil, nil, domain.NewValidationError("ratings", fmt.Sprintf("expected %d ratings", len(bw.Jokes)))
	}

	return s.repo.RateBatch(ctx, batchID, userID, ratings, feedback)
}

func (s *QCService) QueueCount(ctx context.Context, roundID int64) (int, error) {
	return s.repo.CountSubmittedBatches(ctx, roundID)
}

