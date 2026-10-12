package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"jokefactory/src/core/domain"
)

const roundColumns = `round_id, round_number, status, customer_budget, batch_size,
	market_price, cost_of_publishing, cost_of_discard, customer_count,
	buy_threshold, jitter, swap_margin, feedback_joke_count, feedback_pass_threshold,
	started_at, ended_at, created_at, is_popped_active,
	market_mode, arrival_window_seconds, setup_cost_per_batch,
	carrying_cost_per_joke_minute, wip_limit, preference_base,
	shipping_fee_small, shipping_fee_standard, shipping_fee_large,
	storage_fee_per_joke_minute, hosting_fee_per_joke_minute,
	customers_per_team`

type scannable interface {
	Scan(dest ...any) error
}

func scanRound(row scannable) (*domain.Round, error) {
	var rd domain.Round
	err := row.Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.MarketPrice, &rd.CostOfPublishing, &rd.CostOfDiscard, &rd.CustomerCount,
		&rd.BuyThreshold, &rd.Jitter, &rd.SwapMargin, &rd.FeedbackJokeCount, &rd.FeedbackPassThreshold,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
		&rd.MarketMode, &rd.ArrivalWindowSeconds, &rd.SetupCostPerBatch,
		&rd.CarryingCostPerJokeMinute, &rd.WIPLimit, &rd.PreferenceBase,
		&rd.ShippingFeeSmall, &rd.ShippingFeeStandard, &rd.ShippingFeeLarge,
		&rd.StorageFeePerJokeMinute, &rd.HostingFeePerJokeMinute,
		&rd.CustomersPerTeam,
	)
	if err != nil {
		return nil, err
	}
	return &rd, nil
}

func scanUser(row scannable) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.Status, &u.AssignedAt, &u.JoinedAt, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
