package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

const (
	feedbackTargetGood    = 2
	feedbackTargetImprove = 3
	feedbackTargetTotal   = 5
)

// JokeFeedback is the privacy-safe per-joke feedback projection.
// It never includes dim_fit values, categories, or the ideal profile.
type JokeFeedback struct {
	JokeID            int64
	JokeTitle         string
	WasBought         bool
	GoodDimensions    []string
	ImproveDimensions []string
}

// FeedbackService builds curated Good/Improve dimension signals for a team.
type FeedbackService struct {
	repo ports.Store
	log  *slog.Logger
}

func NewFeedbackService(repo ports.Store, log *slog.Logger) *FeedbackService {
	return &FeedbackService{repo: repo, log: log}
}

// Get returns the latest feedback_joke_count published jokes for the caller's team.
func (s *FeedbackService) Get(ctx context.Context, roundID, teamID, userID int64) ([]JokeFeedback, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.TeamID == nil || *user.TeamID != teamID {
		return nil, domain.NewForbiddenError("user not on this team")
	}
	if user.Role == nil || (*user.Role != domain.RoleJM && *user.Role != domain.RoleMarketing) {
		return nil, domain.NewForbiddenError("user must be JM or MARKETING")
	}

	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive && round.Status != domain.RoundEnded {
		return nil, domain.NewConflictError("round not active")
	}

	rows, err := s.repo.ListTeamFeedbackJokes(ctx, roundID, teamID, round.FeedbackJokeCount)
	if err != nil {
		return nil, err
	}

	out := make([]JokeFeedback, 0, len(rows))
	for _, row := range rows {
		good, improve := SelectFeedbackDimensions(row.DimFits, round.FeedbackPassThreshold)
		out = append(out, JokeFeedback{
			JokeID:            row.JokeID,
			JokeTitle:         row.JokeTitle,
			WasBought:         row.WasBought,
			GoodDimensions:    good,
			ImproveDimensions: improve,
		})
	}
	return out, nil
}

// SelectFeedbackDimensions picks up to 2 Good + 3 Improve dimensions.
// Pass = dim_fit >= threshold. Prefer 3 Improve + 2 Good; backfill from the
// other side when short. Tie-break uses scoring.AllDimensions order.
// Returned names never include fit values.
func SelectFeedbackDimensions(dimFits map[domain.Dimension]float64, threshold float64) (good, improve []string) {
	good = []string{}
	improve = []string{}
	if len(dimFits) == 0 {
		return good, improve
	}

	var pass, fail []domain.Dimension
	for _, dim := range scoring.AllDimensions {
		score, ok := dimFits[dim]
		if !ok {
			continue
		}
		if score >= threshold {
			pass = append(pass, dim)
		} else {
			fail = append(fail, dim)
		}
	}

	improveDims := make([]domain.Dimension, 0, feedbackTargetTotal)
	goodDims := make([]domain.Dimension, 0, feedbackTargetTotal)

	for i := 0; i < len(fail) && len(improveDims) < feedbackTargetImprove; i++ {
		improveDims = append(improveDims, fail[i])
	}
	for i := 0; i < len(pass) && len(goodDims) < feedbackTargetGood; i++ {
		goodDims = append(goodDims, pass[i])
	}

	fi, pi := len(improveDims), len(goodDims)
	for len(improveDims)+len(goodDims) < feedbackTargetTotal {
		if fi < len(fail) {
			improveDims = append(improveDims, fail[fi])
			fi++
			continue
		}
		if pi < len(pass) {
			goodDims = append(goodDims, pass[pi])
			pi++
			continue
		}
		break
	}

	for _, d := range goodDims {
		good = append(good, string(d))
	}
	for _, d := range improveDims {
		improve = append(improve, string(d))
	}
	return good, improve
}
