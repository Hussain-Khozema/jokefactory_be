package usecase

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// AICustomerService generates simulated buyers and evaluates buy/swap decisions.
type AICustomerService struct {
	repo ports.Store
	rng  *rand.Rand
	log  *slog.Logger
}

// NewAICustomerService wires the AI customer engine. rng may be nil (seeded from time).
func NewAICustomerService(repo ports.Store, rng *rand.Rand, log *slog.Logger) *AICustomerService {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // sim RNG, not crypto
	}
	return &AICustomerService{repo: repo, rng: rng, log: log}
}

// GenerateCustomers creates N AI customers for a round with threshold = τ ± jitter.
func (s *AICustomerService) GenerateCustomers(ctx context.Context, round *domain.Round) error {
	if round == nil {
		return domain.NewValidationError("round", "round is required")
	}
	n := round.CustomerCount
	if n <= 0 {
		n = domain.DefaultCustomerCount
	}
	customers := make([]domain.AICustomer, 0, n)
	for i := 0; i < n; i++ {
		jitter := (s.rng.Float64()*2 - 1) * round.Jitter // [-jitter, +jitter]
		customers = append(customers, domain.AICustomer{
			RoundID:           round.ID,
			PersonalThreshold: round.BuyThreshold + jitter,
			StartingBudget:    round.CustomerBudget,
			RemainingBudget:   round.CustomerBudget,
		})
	}
	return s.repo.ReplaceAICustomers(ctx, round.ID, customers)
}

// EvaluatePurchases runs buy/swap for every AI customer against newly classified jokes.
func (s *AICustomerService) EvaluatePurchases(ctx context.Context, roundID int64, jokeIDs []int64) error {
	if len(jokeIDs) == 0 {
		return nil
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return err
	}
	if round.Status != domain.RoundActive {
		return nil
	}

	candidates, err := s.repo.ListCandidateJokes(ctx, jokeIDs)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	customers, err := s.repo.ListAICustomers(ctx, roundID)
	if err != nil {
		return err
	}

	// Shuffle customer order so concurrent-ish fairness is random.
	s.shuffleCustomers(customers)

	for i := range customers {
		if err := s.evaluateOne(ctx, round, &customers[i], candidates); err != nil {
			s.log.Error("ai customer evaluate failed",
				"round_id", roundID,
				"ai_customer_id", customers[i].ID,
				"error", err,
			)
			// Continue other customers; per-purchase txs already committed.
		}
	}
	return nil
}

func (s *AICustomerService) evaluateOne(
	ctx context.Context,
	round *domain.Round,
	customer *domain.AICustomer,
	candidates []ports.CandidateJoke,
) error {
	holdings, err := s.repo.ListHoldings(ctx, round.ID, customer.ID)
	if err != nil {
		return err
	}
	owned := make(map[int64]struct{}, len(holdings))
	for _, h := range holdings {
		owned[h.JokeID] = struct{}{}
	}

	order := s.rng.Perm(len(candidates))
	price := round.MarketPrice
	remaining := customer.RemainingBudget

	for _, idx := range order {
		c := candidates[idx]
		if _, ok := owned[c.JokeID]; ok {
			continue
		}
		if c.TrueFit < customer.PersonalThreshold {
			continue
		}

		if remaining >= price {
			holdings, remaining, err = s.tryBuy(ctx, round.ID, customer.ID, c, price, remaining, owned, holdings)
			if err != nil {
				return err
			}
			continue
		}

		holdings, err = s.trySwap(ctx, round, customer.ID, c, price, owned, holdings)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *AICustomerService) tryBuy(
	ctx context.Context,
	roundID, customerID int64,
	c ports.CandidateJoke,
	price, remaining float64,
	owned map[int64]struct{},
	holdings []ports.HeldJoke,
) ([]ports.HeldJoke, float64, error) {
	if err := s.repo.BuyJoke(ctx, roundID, customerID, c.JokeID, c.TeamID, price); err != nil {
		if domain.IsConflict(err) {
			return holdings, remaining, nil
		}
		return holdings, remaining, err
	}
	owned[c.JokeID] = struct{}{}
	holdings = append(holdings, ports.HeldJoke{
		JokeID: c.JokeID, TeamID: c.TeamID, TrueFit: c.TrueFit, Price: price,
	})
	return holdings, remaining - price, nil
}

func (s *AICustomerService) trySwap(
	ctx context.Context,
	round *domain.Round,
	customerID int64,
	c ports.CandidateJoke,
	price float64,
	owned map[int64]struct{},
	holdings []ports.HeldJoke,
) ([]ports.HeldJoke, error) {
	weakIdx := weakestHoldingIndex(holdings, s.rng)
	if weakIdx < 0 {
		return holdings, nil
	}
	weak := holdings[weakIdx]
	if c.TrueFit <= weak.TrueFit+round.SwapMargin {
		return holdings, nil
	}
	if err := s.repo.SwapJoke(
		ctx, round.ID, customerID,
		c.JokeID, c.TeamID, weak.JokeID, weak.TeamID, price,
	); err != nil {
		if domain.IsConflict(err) {
			return holdings, nil
		}
		return holdings, err
	}
	delete(owned, weak.JokeID)
	owned[c.JokeID] = struct{}{}
	holdings[weakIdx] = ports.HeldJoke{
		JokeID: c.JokeID, TeamID: c.TeamID, TrueFit: c.TrueFit, Price: price,
	}
	return holdings, nil
}

func weakestHoldingIndex(holdings []ports.HeldJoke, rng *rand.Rand) int {
	if len(holdings) == 0 {
		return -1
	}
	minFit := holdings[0].TrueFit
	for i := 1; i < len(holdings); i++ {
		if holdings[i].TrueFit < minFit {
			minFit = holdings[i].TrueFit
		}
	}
	var idxs []int
	for i := range holdings {
		if holdings[i].TrueFit == minFit {
			idxs = append(idxs, i)
		}
	}
	return idxs[rng.Intn(len(idxs))]
}

func (s *AICustomerService) shuffleCustomers(customers []domain.AICustomer) {
	s.rng.Shuffle(len(customers), func(i, j int) {
		customers[i], customers[j] = customers[j], customers[i]
	})
}

// Market lists published jokes with sold_count and team labels.
func (s *AICustomerService) Market(ctx context.Context, userID, roundID int64) ([]ports.MarketJoke, error) {
	if _, err := s.repo.GetUserByID(ctx, userID); err != nil {
		return nil, err
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive && round.Status != domain.RoundEnded {
		return nil, domain.NewConflictError("ROUND_NOT_ACTIVE")
	}
	return s.repo.ListMarket(ctx, roundID)
}
