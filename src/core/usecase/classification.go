package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

// ClassificationService processes published batches: Length + LLM classify → fit.
type ClassificationService struct {
	repo       ports.Store
	classifier ports.Classifier
	purchases  PurchaseEvaluator
	modelName  string
	log        *slog.Logger
}

// PurchaseEvaluator is called after fit materialization (AI customer buy/swap).
type PurchaseEvaluator interface {
	EvaluatePurchases(ctx context.Context, roundID int64, jokeIDs []int64) error
}

// NewClassificationService wires the classification pipeline worker logic.
func NewClassificationService(
	repo ports.Store,
	classifier ports.Classifier,
	purchases PurchaseEvaluator,
	modelName string,
	log *slog.Logger,
) *ClassificationService {
	if modelName == "" {
		modelName = "stub"
	}
	return &ClassificationService{
		repo:       repo,
		classifier: classifier,
		purchases:  purchases,
		modelName:  modelName,
		log:        log,
	}
}

// ProcessBatch classifies published jokes in a batch and materializes fit scores.
func (s *ClassificationService) ProcessBatch(ctx context.Context, batchID int64) error {
	job, err := s.claimOrEnsure(ctx, batchID)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}

	if err := s.run(ctx, job); err != nil {
		s.log.Error("classification failed",
			"batch_id", batchID,
			"attempt", job.Attempts,
			"error", err,
		)
		if markErr := s.repo.MarkClassificationFailed(ctx, batchID, err.Error()); markErr != nil {
			return fmt.Errorf("classify: %w (also failed to mark failed: %w)", err, markErr)
		}
		return err
	}
	return nil
}

func (s *ClassificationService) claimOrEnsure(ctx context.Context, batchID int64) (*domain.ClassificationJob, error) {
	job, err := s.repo.ClaimClassificationJob(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if job != nil {
		return job, nil
	}
	// Ensure a job exists for reconciler-discovered orphans, then retry claim.
	batch, getErr := s.repo.GetBatchWithJokes(ctx, batchID)
	if getErr != nil {
		return nil, getErr
	}
	if err := s.repo.EnsureClassificationJob(ctx, batchID, batch.Batch.RoundID); err != nil {
		return nil, err
	}
	return s.repo.ClaimClassificationJob(ctx, batchID)
}

func (s *ClassificationService) run(ctx context.Context, job *domain.ClassificationJob) error {
	batch, err := s.repo.GetBatchWithJokes(ctx, job.BatchID)
	if err != nil {
		return err
	}

	published := publishedJokes(batch.Jokes)
	if len(published) == 0 {
		return s.repo.MarkClassificationDone(ctx, job.BatchID, s.modelName)
	}

	profile, err := s.repo.GetIdealProfile(ctx, job.RoundID)
	if err != nil {
		return err
	}
	if err := scoring.ValidateIdealProfile(profile); err != nil {
		return err
	}

	fits, err := s.classifyAndScore(ctx, job.RoundID, published, profile)
	if err != nil {
		return err
	}
	if err := s.repo.PersistJokeFits(ctx, fits); err != nil {
		return err
	}
	if err := s.repo.MarkClassificationDone(ctx, job.BatchID, s.modelName); err != nil {
		return err
	}
	if s.purchases != nil {
		ids := make([]int64, len(fits))
		for i := range fits {
			ids[i] = fits[i].JokeID
		}
		if err := s.purchases.EvaluatePurchases(ctx, job.RoundID, ids); err != nil {
			s.log.Error("purchase evaluation failed after classification",
				"batch_id", job.BatchID,
				"round_id", job.RoundID,
				"error", err,
			)
			// Fit is already persisted; purchases can be retried via future reconcile.
		}
	}
	return nil
}

func (s *ClassificationService) classifyAndScore(
	ctx context.Context,
	roundID int64,
	published []domain.Joke,
	profile domain.IdealProfile,
) ([]ports.JokeFitMaterialization, error) {
	inputs := make([]ports.JokeInput, 0, len(published))
	for _, j := range published {
		title := ""
		if j.Title != nil {
			title = *j.Title
		}
		inputs = append(inputs, ports.JokeInput{
			JokeID: j.ID,
			Text:   j.Text,
			Title:  title,
		})
	}

	llmResults, err := s.classifier.Classify(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("classifier: %w", err)
	}
	byID := make(map[int64]ports.JokeClassification, len(llmResults))
	for _, r := range llmResults {
		byID[r.JokeID] = r
	}

	scoringProfile := scoring.IdealProfile(profile)
	fits := make([]ports.JokeFitMaterialization, 0, len(published))
	for i := range published {
		fit, err := materializeJokeFit(&published[i], roundID, byID, scoringProfile)
		if err != nil {
			return nil, err
		}
		fits = append(fits, fit)
	}
	return fits, nil
}

func materializeJokeFit(
	j *domain.Joke,
	roundID int64,
	byID map[int64]ports.JokeClassification,
	profile scoring.IdealProfile,
) (ports.JokeFitMaterialization, error) {
	llm, ok := byID[j.ID]
	if !ok {
		return ports.JokeFitMaterialization{}, fmt.Errorf("classifier missing result for joke %d", j.ID)
	}
	classification := scoring.Classification{}
	classification[domain.DimLength] = scoring.ClassifyLength(j.Text)
	for _, dim := range scoring.LLMDimensions() {
		cat, ok := llm.Categories[dim]
		if !ok || cat == "" {
			return ports.JokeFitMaterialization{}, fmt.Errorf("classifier missing %s for joke %d", dim, j.ID)
		}
		if !scoring.IsValidCategory(dim, cat) {
			return ports.JokeFitMaterialization{}, fmt.Errorf("invalid category %q for %s (joke %d)", cat, dim, j.ID)
		}
		classification[dim] = cat
	}

	dimFits := make(map[domain.Dimension]float64, len(scoring.AllDimensions))
	for _, dim := range scoring.AllDimensions {
		dimFits[dim] = scoring.DimFit(dim, profile[dim], classification[dim])
	}
	return ports.JokeFitMaterialization{
		JokeID:     j.ID,
		RoundID:    roundID,
		Categories: classification,
		DimFits:    dimFits,
		TrueFit:    scoring.TrueFit(classification, profile),
	}, nil
}

func publishedJokes(jokes []domain.Joke) []domain.Joke {
	out := make([]domain.Joke, 0, len(jokes))
	for _, j := range jokes {
		if j.PublishStatus == domain.PublishPublished {
			out = append(out, j)
		}
	}
	return out
}
