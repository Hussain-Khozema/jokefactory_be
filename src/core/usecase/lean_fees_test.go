package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

// Admin login creates both rounds with the Lean rule set: Round 1 batches of
// exactly 5, Round 2 batches of up to 10. Reset Game puts both back, whatever
// the instructor changed, and keeps the instructor.
func TestLoginAndResetGiveRound1FiveAndRound2Ten(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	admin := usecase.NewAdminAuthService(st, "pw")
	if _, err := admin.Login(ctx, "Instructor", "pw"); err != nil {
		t.Fatal(err)
	}
	checkRounds := func(when string) {
		t.Helper()
		for _, want := range []struct {
			id        int64
			batchSize int
		}{{1, 5}, {2, 10}} {
			rd, err := st.GetRoundByID(ctx, want.id)
			if err != nil {
				t.Fatalf("%s: round %d: %v", when, want.id, err)
			}
			if rd.BatchSize != want.batchSize || rd.Status != domain.RoundConfigured || rd.MarketMode != domain.MarketLean ||
				rd.ShippingFeeStandard != 0 || rd.StorageFeePerJokeMinute != 0 || rd.CostOfDiscard != 0 ||
				rd.CostOfPublishing != 0.1 || rd.HostingFeePerJokeMinute != 0.05 || rd.StartedAt != nil {
				t.Fatalf("%s: round %d = %+v; want CONFIGURED LEAN, batch size %d, the two costs",
					when, want.id, rd, want.batchSize)
			}
		}
	}
	checkRounds("after login")

	instructor := usecase.NewInstructorService(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	changed := domain.DefaultRoundConfigFor(2)
	changed.BatchSize = 4
	changed.StorageFeePerJokeMinute = 0.5
	if _, err := instructor.Config(ctx, 2, &changed, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	round1 := domain.DefaultRoundConfig()
	if _, err := instructor.Config(ctx, 1, &round1, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}

	if err := admin.ResetGame(ctx); err != nil {
		t.Fatal(err)
	}
	checkRounds("after reset")
	if _, err := st.GetUserByDisplayName(ctx, "Instructor"); err != nil {
		t.Fatalf("the instructor did not survive the reset: %v", err)
	}
	if profile, _ := st.GetIdealProfile(ctx, 1); len(profile) != 0 {
		t.Fatalf("reset kept round 1's ideal profile: %v", profile)
	}
}

func TestConfigRejectsBadLeanFees(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		mutate      func(*domain.RoundConfig)
	}{
		{"small negative", "shipping_fee_small", func(c *domain.RoundConfig) { c.ShippingFeeSmall = -0.01 }},
		{"standard negative", "shipping_fee_standard", func(c *domain.RoundConfig) { c.ShippingFeeStandard = -1 }},
		{"large negative", "shipping_fee_large", func(c *domain.RoundConfig) { c.ShippingFeeLarge = -3 }},
		{"storage negative", "storage_fee_per_joke_minute", func(c *domain.RoundConfig) { c.StorageFeePerJokeMinute = -0.1 }},
		{"hosting negative", "hosting_fee_per_joke_minute", func(c *domain.RoundConfig) { c.HostingFeePerJokeMinute = -0.05 }},
		{"small above standard", "shipping_fee_standard", func(c *domain.RoundConfig) { c.ShippingFeeSmall = 2.5 }},
		{"standard above large", "shipping_fee_standard", func(c *domain.RoundConfig) { c.ShippingFeeStandard = 3.5 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, svc := newLeanConfigInstructor()
			cfg := domain.DefaultRoundConfig()
			tc.mutate(&cfg)
			_, err := svc.Config(context.Background(), 1, &cfg, nil)
			var de *domain.DomainError
			if !domain.IsValidationError(err) || !errors.As(err, &de) || de.Field != tc.field {
				t.Fatalf("want a validation error on %s, got %v", tc.field, err)
			}
			if _, ok := st.Rounds[1]; ok {
				t.Fatal("a rejected config was written")
			}
		})
	}
}

func TestConfigAcceptsFlatAndFreeShipping(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	for _, fees := range [][3]float64{{2, 2, 2}, {0, 0, 0}, {0.5, 1.25, 7}} {
		cfg := domain.DefaultRoundConfig()
		cfg.ShippingFeeSmall, cfg.ShippingFeeStandard, cfg.ShippingFeeLarge = fees[0], fees[1], fees[2]
		cfg.StorageFeePerJokeMinute, cfg.HostingFeePerJokeMinute = 0, 0
		res, err := svc.Config(context.Background(), 1, &cfg, nil)
		if err != nil {
			t.Fatalf("fees %v: %v", fees, err)
		}
		got := domain.ConfigFromRound(res.Round)
		if got.ShippingFeeSmall != fees[0] || got.ShippingFeeStandard != fees[1] || got.ShippingFeeLarge != fees[2] {
			t.Fatalf("fees %v not persisted: %+v", fees, got)
		}
	}
}

func TestMergeConfigLeanFees(t *testing.T) {
	base := domain.DefaultRoundConfig()
	small, standard, large, storage, hosting := 0.5, 1.5, 2.5, 0.2, 0.01
	got := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{
		ShippingFeeSmall: &small, ShippingFeeStandard: &standard, ShippingFeeLarge: &large,
		StorageFeePerJokeMinute: &storage, HostingFeePerJokeMinute: &hosting,
	})
	if got.ShippingFeeSmall != 0.5 || got.ShippingFeeStandard != 1.5 || got.ShippingFeeLarge != 2.5 ||
		got.StorageFeePerJokeMinute != 0.2 || got.HostingFeePerJokeMinute != 0.01 {
		t.Fatalf("fee overlay not merged: %+v", got)
	}
	if kept := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{}); kept != base {
		t.Fatalf("nil overlay changed config: %+v", kept)
	}
}

// startedDefaultRound configures round 1 with the defaults and the five fees
// (so every fee has a value to change) and starts it.
func startedDefaultRound(t *testing.T, svc *usecase.InstructorService) domain.RoundConfig {
	t.Helper()
	ctx := context.Background()
	cfg := testutil.FiveFees(domain.DefaultRoundConfig())
	if _, err := svc.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	// The start sized the market (customers_per_team × 1 team): the round's
	// config from here on is what was stored at the start.
	return domain.ConfigFromRound(started)
}

func TestConfigLocksLeanFeesOnceStarted(t *testing.T) {
	for name, mutate := range map[string]func(*domain.RoundConfig){
		"shipping small":    func(c *domain.RoundConfig) { c.ShippingFeeSmall = 0.5 },
		"shipping standard": func(c *domain.RoundConfig) { c.ShippingFeeStandard = 2.5 },
		"shipping large":    func(c *domain.RoundConfig) { c.ShippingFeeLarge = 4 },
		"storage":           func(c *domain.RoundConfig) { c.StorageFeePerJokeMinute = 0.2 },
		"hosting":           func(c *domain.RoundConfig) { c.HostingFeePerJokeMinute = 0.1 },
		"listing":           func(c *domain.RoundConfig) { c.CostOfPublishing = 0.25 },
		"discard":           func(c *domain.RoundConfig) { c.CostOfDiscard = 0.05 },
	} {
		t.Run(name, func(t *testing.T) {
			_, svc := newLeanConfigInstructor()
			cfg := startedDefaultRound(t, svc)
			want := cfg
			mutate(&cfg)
			_, err := svc.Config(context.Background(), 1, &cfg, nil)
			var de *domain.DomainError
			if !domain.IsConflict(err) || !errors.As(err, &de) || !strings.Contains(de.Message, "MARKET_MODE_LOCKED") {
				t.Fatalf("want a MARKET_MODE_LOCKED conflict, got %v", err)
			}
			rd, err := svc.GetRound(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := domain.ConfigFromRound(rd); got != want {
				t.Fatalf("a rejected fee change was written: %+v", got)
			}
		})
	}
}

func TestConfigAllowsResendingSameLeanFeesOnceStarted(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	cfg := startedDefaultRound(t, svc)
	// Values that round to the stored scale are the same values.
	cfg.ShippingFeeStandard = 2.000001
	cfg.StorageFeePerJokeMinute = 0.050000001
	cfg.CostOfPublishing = 0.1000001
	cfg.CostOfDiscard = 0.010000001
	cfg.MarketPrice = 2
	if _, err := svc.Config(context.Background(), 1, &cfg, nil); err != nil {
		t.Fatalf("resending the same fees on an ACTIVE round: %v", err)
	}
	rd, err := svc.GetRound(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if rd.MarketPrice != 2 {
		t.Fatalf("price change not written: %+v", rd)
	}
}

func TestConfigAllowsLeanFeeChangesBeforeStart(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	cfg := domain.DefaultRoundConfig()
	if _, err := svc.Config(context.Background(), 1, &cfg, nil); err != nil {
		t.Fatal(err)
	}
	cfg.ShippingFeeLarge = 5
	cfg.HostingFeePerJokeMinute = 0.2
	cfg.CostOfPublishing = 0.25
	cfg.CostOfDiscard = 0.05
	res, err := svc.Config(context.Background(), 1, &cfg, nil)
	if err != nil {
		t.Fatalf("fee change on a CONFIGURED round: %v", err)
	}
	if res.Round.ShippingFeeLarge != 5 || res.Round.HostingFeePerJokeMinute != 0.2 ||
		res.Round.CostOfPublishing != 0.25 || res.Round.CostOfDiscard != 0.05 {
		t.Fatalf("fee change not written: %+v", res.Round)
	}
}
