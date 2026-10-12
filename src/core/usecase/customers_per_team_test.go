package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"strings"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

// startWithTeams configures round 1 with cfg, assigns `teams` teams (two
// participants each, so every team has a JM and a Marketing) and starts it.
// It returns the started round and the number of AI customers generated.
func startWithTeams(t *testing.T, cfg domain.RoundConfig, teams int) (*domain.Round, int, *testutil.Store) {
	t.Helper()
	ctx := context.Background()
	st := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ai := usecase.NewAICustomerService(st, rand.New(rand.NewSource(1)), log) //nolint:gosec
	instructor := usecase.NewInstructorService(st, ai, log)
	session := usecase.NewSessionService(st, log)

	if _, err := st.InsertRoundConfig(ctx, 1, &cfg); err != nil {
		t.Fatal(err)
	}
	if teams > 0 {
		names := make([]string, 0, 2*teams)
		for i := 0; i < 2*teams; i++ {
			names = append(names, "P"+string(rune('A'+i)))
		}
		joinMany(t, session, names)
		if _, err := instructor.Assign(ctx, 1, teams); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	round, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	customers, err := st.ListAICustomers(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	return round, len(customers), st
}

// Spec §3: a LEAN round with customers_per_team > 0 gets customers_per_team ×
// T customers, T being the teams with a JM or Marketing assigned at start,
// and the round's customer_count says so (returned and stored).
func TestStartRoundLeanCustomersPerTeam(t *testing.T) {
	for _, tc := range []struct {
		name          string
		perTeam       int
		teams         int
		mode          domain.MarketMode
		wantCustomers int
	}{
		{"three teams", 18, 3, domain.MarketLean, 54},
		{"no teams counts as one", 18, 0, domain.MarketLean, 18},
		{"zero keeps the fixed count", 0, 3, domain.MarketLean, 40},
		{"classic keeps the fixed count", 18, 3, domain.MarketClassic, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := domain.DefaultRoundConfig()
			cfg.MarketMode = tc.mode
			cfg.CustomerCount = 40
			cfg.CustomersPerTeam = tc.perTeam
			round, generated, st := startWithTeams(t, cfg, tc.teams)
			if generated != tc.wantCustomers || round.CustomerCount != tc.wantCustomers {
				t.Fatalf("generated %d customers, returned customer_count %d; want %d",
					generated, round.CustomerCount, tc.wantCustomers)
			}
			stored, err := st.GetRoundByID(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if stored.CustomerCount != tc.wantCustomers || stored.CustomersPerTeam != tc.perTeam ||
				stored.Status != domain.RoundActive {
				t.Fatalf("stored round %+v; want ACTIVE, customer_count %d, customers_per_team %d",
					stored, tc.wantCustomers, tc.perTeam)
			}
		})
	}
}

// A team counts only when someone on it is assigned as JM or Marketing:
// teams left empty by Assign (more teams than participants) do not.
func TestStartRoundCountsOnlyStaffedTeams(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ai := usecase.NewAICustomerService(st, rand.New(rand.NewSource(1)), log) //nolint:gosec
	instructor := usecase.NewInstructorService(st, ai, log)
	session := usecase.NewSessionService(st, log)
	cfg := domain.DefaultRoundConfig()
	if _, err := st.InsertRoundConfig(ctx, 1, &cfg); err != nil {
		t.Fatal(err)
	}
	joinMany(t, session, []string{"A", "B", "C"}) // teams 1 and 2 staffed, 3 and 4 empty
	if _, err := instructor.Assign(ctx, 1, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	round, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if round.CustomerCount != 2*domain.DefaultCustomersPerTeam {
		t.Fatalf("customer_count %d; want %d (two staffed teams)", round.CustomerCount, 2*domain.DefaultCustomersPerTeam)
	}
}

func TestConfigValidatesCustomersPerTeam(t *testing.T) {
	for _, v := range []int{-1, 1001} {
		cfg := domain.DefaultRoundConfig()
		cfg.CustomersPerTeam = v
		_, svc := newLeanConfigInstructor()
		_, err := svc.Config(context.Background(), 1, &cfg, nil)
		var de *domain.DomainError
		if !errors.As(err, &de) || de.Field != "customers_per_team" {
			t.Fatalf("customers_per_team %d: %v; want a validation error on customers_per_team", v, err)
		}
	}
	for _, v := range []int{0, 1000} {
		cfg := domain.DefaultRoundConfig()
		cfg.CustomersPerTeam = v
		_, svc := newLeanConfigInstructor()
		if _, err := svc.Config(context.Background(), 1, &cfg, nil); err != nil {
			t.Fatalf("customers_per_team %d: %v", v, err)
		}
	}
}

func TestConfigLocksCustomersPerTeamOnceStarted(t *testing.T) {
	ctx := context.Background()
	_, svc := newLeanConfigInstructor()
	startedDefaultRound(t, svc)
	rd, err := svc.GetRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := domain.ConfigFromRound(rd)
	cfg.CustomersPerTeam = 20
	_, err = svc.Config(ctx, 1, &cfg, nil)
	var de *domain.DomainError
	if !domain.IsConflict(err) || !errors.As(err, &de) || !strings.Contains(de.Message, "MARKET_MODE_LOCKED") {
		t.Fatalf("want a MARKET_MODE_LOCKED conflict, got %v", err)
	}
	cfg.CustomersPerTeam = domain.DefaultCustomersPerTeam
	if _, err := svc.Config(ctx, 1, &cfg, nil); err != nil {
		t.Fatalf("re-sending the stored customers_per_team: %v", err)
	}
}

func TestMergeConfigCustomersPerTeam(t *testing.T) {
	base := domain.DefaultRoundConfig()
	if got := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{}); got.CustomersPerTeam != 18 {
		t.Fatalf("nil overlay changed customers_per_team to %d", got.CustomersPerTeam)
	}
	zero := 0
	if got := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{CustomersPerTeam: &zero}); got.CustomersPerTeam != 0 {
		t.Fatalf("customers_per_team %d; want 0", got.CustomersPerTeam)
	}
}
