package usecase

import (
	"context"
	"fmt"
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

// Split cuts a Joke Maker's unsplit raw blob into individual jokes. This is the
// first point at which a joke count exists on the raw path, so the R1/R2
// batch-size rules are enforced here rather than in BatchService.Submit.
func (s *MarketingService) Split(
	ctx context.Context,
	userID, batchID int64,
	jokes []string,
) (*MarketingQueueItem, error) {
	user, round, err := s.requireEditableBatch(ctx, userID, batchID)
	if err != nil {
		return nil, err
	}

	cleaned := make([]string, 0, len(jokes))
	for _, j := range jokes {
		if t := strings.TrimSpace(j); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) == 0 {
		return nil, domain.NewValidationError("jokes", "at least one joke required")
	}

	// These two rules moved here from BatchService.Submit: a raw blob carries no
	// joke count, so the split is the first place they can be applied. The
	// message strings match the ones Submit used, so the frontend sees no change.
	if round.RoundNumber == 1 && len(cleaned) != round.BatchSize {
		return nil, domain.NewValidationError("jokes", fmt.Sprintf("expected %d jokes", round.BatchSize))
	} else if round.RoundNumber >= 2 && len(cleaned) > round.BatchSize {
		return nil, domain.NewValidationError("jokes", fmt.Sprintf("expected up to %d jokes", round.BatchSize))
	}

	split, err := s.repo.SplitBatch(ctx, batchID, user.ID, *user.TeamID, cleaned)
	if err != nil {
		return nil, err
	}
	return s.queueItem(ctx, split, round.ID, *user.TeamID)
}

// requireEditableBatch resolves the marketer and round for a batch the marketer
// is allowed to edit: same team, ACTIVE round, SUBMITTED batch, and the lock
// held by this marketer. An expired lock must not let a second marketer
// overwrite the first marketer's work, so the holder is checked explicitly.
func (s *MarketingService) requireEditableBatch(
	ctx context.Context,
	userID, batchID int64,
) (*domain.User, *domain.Round, error) {
	user, err := s.requireMarketer(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	existing, err := s.repo.GetBatchWithJokes(ctx, batchID)
	if err != nil {
		return nil, nil, err
	}
	if existing.Batch.TeamID != *user.TeamID {
		return nil, nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	round, err := s.repo.GetRoundByID(ctx, existing.Batch.RoundID)
	if err != nil {
		return nil, nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, nil, domain.NewConflictError("ROUND_NOT_ACTIVE")
	}
	if existing.Batch.Status == domain.BatchProcessed {
		return nil, nil, domain.NewConflictError("BATCH_ALREADY_PROCESSED")
	}
	if existing.Batch.Status != domain.BatchSubmitted {
		return nil, nil, domain.NewConflictError("batch not submitted")
	}
	if existing.Batch.LockedBy == nil || *existing.Batch.LockedBy != user.ID {
		return nil, nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	return user, round, nil
}

// queueItem wraps a batch in the same {batch, jokes, queue_size} envelope
// QueueNext returns, so the frontend can drop either response into queue state.
func (s *MarketingService) queueItem(
	ctx context.Context,
	bwj *ports.BatchWithJokes,
	roundID, teamID int64,
) (*MarketingQueueItem, error) {
	count, err := s.repo.CountSubmittedBatchesForTeam(ctx, roundID, teamID)
	if err != nil {
		return nil, err
	}
	return &MarketingQueueItem{Batch: bwj.Batch, Jokes: bwj.Jokes, QueueSize: count}, nil
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
