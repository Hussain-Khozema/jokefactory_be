package usecase

import (
	"context"
	"log/slog"
	"strings"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// MarketingService handles Marketing queue claim and publish/discard.
type MarketingService struct {
	repo       ports.Store
	dispatcher ports.ClassificationDispatcher
	log        *slog.Logger
}

func NewMarketingService(repo ports.Store, dispatcher ports.ClassificationDispatcher, log *slog.Logger) *MarketingService {
	return &MarketingService{repo: repo, dispatcher: dispatcher, log: log}
}

type MarketingQueueItem struct {
	Batch     domain.Batch
	Jokes     []domain.Joke
	QueueSize int
}

// QueueNext claims the next SUBMITTED batch for the marketer's team.
// Returns nil when the queue is empty.
func (s *MarketingService) QueueNext(ctx context.Context, userID, roundID int64) (*MarketingQueueItem, error) {
	user, err := s.requireMarketer(ctx, userID)
	if err != nil {
		return nil, err
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, domain.NewConflictError("ROUND_NOT_ACTIVE")
	}

	claimed, err := s.repo.ClaimNextBatch(ctx, roundID, *user.TeamID, user.ID)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountSubmittedBatchesForTeam(ctx, roundID, *user.TeamID)
	if err != nil {
		return nil, err
	}
	if claimed == nil {
		return &MarketingQueueItem{QueueSize: count}, nil
	}
	return &MarketingQueueItem{
		Batch:     claimed.Batch,
		Jokes:     claimed.Jokes,
		QueueSize: count,
	}, nil
}

// QueueCount returns SUBMITTED batches waiting for the marketer's team.
func (s *MarketingService) QueueCount(ctx context.Context, userID, roundID int64) (int, error) {
	user, err := s.requireMarketer(ctx, userID)
	if err != nil {
		return 0, err
	}
	return s.repo.CountSubmittedBatchesForTeam(ctx, roundID, *user.TeamID)
}

// Publish titles jokes and publishes/discards them for a claimed batch.
func (s *MarketingService) Publish(
	ctx context.Context,
	userID, batchID int64,
	decisions []ports.JokePublishDecision,
) (*ports.PublishResult, error) {
	user, err := s.requireMarketer(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(decisions) == 0 {
		return nil, domain.NewValidationError("jokes", "at least one joke decision required")
	}

	existing, err := s.repo.GetBatchWithJokes(ctx, batchID)
	if err != nil {
		return nil, err
	}
	round, err := s.repo.GetRoundByID(ctx, existing.Batch.RoundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, domain.NewConflictError("ROUND_NOT_ACTIVE")
	}

	normalized := make([]ports.JokePublishDecision, len(decisions))
	for i, d := range decisions {
		normalized[i] = ports.JokePublishDecision{
			JokeID:      d.JokeID,
			Title:       strings.TrimSpace(d.Title),
			IsPublished: d.IsPublished,
		}
	}

	result, err := s.repo.PublishBatch(ctx, batchID, user.ID, *user.TeamID, normalized)
	if err != nil {
		return nil, err
	}

	if err := s.dispatcher.Enqueue(ctx, batchID); err != nil {
		s.log.Error("dispatcher enqueue failed", "batch_id", batchID, "error", err)
	}
	return result, nil
}

func (s *MarketingService) requireMarketer(ctx context.Context, userID int64) (*domain.User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Role == nil || *user.Role != domain.RoleMarketing {
		return nil, domain.NewForbiddenError("user must be MARKETING")
	}
	if user.TeamID == nil {
		return nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	return user, nil
}
