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

// Hint directions for a missed ordinal dimension: move toward the start of
// the dimension's scale (LOWER: shorter, simpler, calmer…) or its end (HIGHER).
const (
	HintLower  = "LOWER"
	HintHigher = "HIGHER"
)

// JokeFeedback is the privacy-safe per-joke feedback projection.
// It never includes dim_fit values, categories, or the ideal profile. For a
// missed dimension on an ordered scale it may carry a direction (ImproveHints):
// which way to move, never how far.
type JokeFeedback struct {
	JokeID            int64
	JokeTitle         string
	WasBought         bool
	GoodDimensions    []string
	ImproveDimensions []string
	ImproveHints      map[string]string
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
	// Without the ideal profile the feedback still works; it just carries no hints.
	ideal, err := s.repo.GetIdealProfile(ctx, roundID)
	if err != nil {
		s.log.Warn("feedback: no ideal profile, sending no hints", "round_id", roundID, "error", err)
		ideal = nil
	}

	out := make([]JokeFeedback, 0, len(rows))
	for _, row := range rows {
		good, improve := SelectFeedbackDimensionsFrom(row.DimFits, round.FeedbackPassThreshold, FeedbackStart(row.JokeID))
		hints := map[string]string{}
		for _, name := range improve {
			dim := domain.Dimension(name)
			if h := DirectionHint(dim, ideal[dim], row.Categories[dim]); h != "" {
				hints[name] = h
			}
		}
		out = append(out, JokeFeedback{
			JokeID:            row.JokeID,
			JokeTitle:         row.JokeTitle,
			WasBought:         row.WasBought,
			GoodDimensions:    good,
			ImproveDimensions: improve,
			ImproveHints:      hints,
		})
	}
	return out, nil
}

// FeedbackStart picks where a joke's walk through the 12 dimensions begins.
// Starting every joke at Length made jokes that missed the same early
// dimensions show identical feedback, and later dimensions (Structure through
// Title Fit) almost never appeared. Starting at jokeID mod 12 varies it from
// joke to joke while staying stable across reloads.
func FeedbackStart(jokeID int64) int {
	n := int64(len(scoring.AllDimensions))
	return int(((jokeID % n) + n) % n)
}

// DirectionHint says which way a missed ordinal dimension should move toward
// the ideal: HintLower (toward the first category of its scale) or HintHigher.
// It is "" for non-ordinal dimensions, unknown categories, the catch-all, or a
// joke already on the ideal.
func DirectionHint(dim domain.Dimension, ideal, joke string) string {
	spec, ok := scoring.Specs[dim]
	if !ok || spec.Scoring != scoring.Ordinal || ideal == "" || joke == "" {
		return ""
	}
	if ideal == scoring.CatchAll || joke == scoring.CatchAll {
		return ""
	}
	ii, ji := -1, -1
	for i, c := range spec.Categories {
		if c == ideal {
			ii = i
		}
		if c == joke {
			ji = i
		}
	}
	switch {
	case ii < 0 || ji < 0 || ii == ji:
		return ""
	case ji > ii:
		return HintLower
	default:
		return HintHigher
	}
}

// SelectFeedbackDimensions is SelectFeedbackDimensionsFrom starting at Length.
func SelectFeedbackDimensions(dimFits map[domain.Dimension]float64, threshold float64) (good, improve []string) {
	return SelectFeedbackDimensionsFrom(dimFits, threshold, 0)
}

// SelectFeedbackDimensionsFrom picks up to 2 Good + 3 Improve dimensions.
// Pass = dim_fit >= threshold. Prefer 3 Improve + 2 Good; backfill from the
// other side when short. Dimensions are taken in scoring.AllDimensions order
// starting at index start and wrapping around. Returned names never include
// fit values.
func SelectFeedbackDimensionsFrom(dimFits map[domain.Dimension]float64, threshold float64, start int) (good, improve []string) {
	good = []string{}
	improve = []string{}
	if len(dimFits) == 0 {
		return good, improve
	}

	n := len(scoring.AllDimensions)
	start = ((start % n) + n) % n
	var pass, fail []domain.Dimension
	for k := 0; k < n; k++ {
		dim := scoring.AllDimensions[(start+k)%n]
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
