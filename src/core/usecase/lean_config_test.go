package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

func newLeanConfigInstructor() (*testutil.Store, *usecase.InstructorService) {
	st := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return st, usecase.NewInstructorService(st, nil, log)
}

func TestConfigPersistsLeanSettings(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	cfg := domain.DefaultRoundConfig()
	cfg.MarketMode = domain.MarketLean
	cfg.ArrivalWindowSeconds = 300
	cfg.SetupCostPerBatch = 3
	cfg.CarryingCostPerJokeMinute = 0.02
	cfg.WIPLimit = 2
	cfg.PreferenceBase = 3

	res, err := svc.Config(context.Background(), 1, &cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := domain.ConfigFromRound(res.Round)
	if got.MarketMode != domain.MarketLean || got.ArrivalWindowSeconds != 300 || got.SetupCostPerBatch != 3 ||
		got.CarryingCostPerJokeMinute != 0.02 || got.WIPLimit != 2 || got.PreferenceBase != 3 {
		t.Fatalf("lean settings not persisted: %+v", got)
	}
}

func TestConfigRejectsBadLeanSettings(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	for name, mutate := range map[string]func(*domain.RoundConfig){
		"window":           func(c *domain.RoundConfig) { c.ArrivalWindowSeconds = 0 },
		"setup":            func(c *domain.RoundConfig) { c.SetupCostPerBatch = -1 },
		"carrying":         func(c *domain.RoundConfig) { c.CarryingCostPerJokeMinute = -0.1 },
		"wip":              func(c *domain.RoundConfig) { c.WIPLimit = -1 },
		"base":             func(c *domain.RoundConfig) { c.PreferenceBase = 1 },
		"base_rounds_to_1": func(c *domain.RoundConfig) { c.PreferenceBase = 1.001 },
		"mode":             func(c *domain.RoundConfig) { c.MarketMode = "SIDEWAYS" },
	} {
		cfg := domain.DefaultRoundConfig()
		mutate(&cfg)
		if _, err := svc.Config(context.Background(), 1, &cfg, nil); !domain.IsValidationError(err) {
			t.Fatalf("%s: want validation error, got %v", name, err)
		}
	}

	cfg := domain.DefaultRoundConfig()
	cfg.MarketMode = "SIDEWAYS"
	_, err := svc.Config(context.Background(), 1, &cfg, nil)
	var de *domain.DomainError
	if !errors.As(err, &de) || de.Field != "market_mode" {
		t.Fatalf("want market_mode validation error, got %v", err)
	}
}

func TestConfigNormalisesMarketMode(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	for raw, want := range map[domain.MarketMode]domain.MarketMode{
		"":     domain.MarketClassic,
		"lean": domain.MarketLean,
	} {
		cfg := domain.DefaultRoundConfig()
		cfg.MarketMode = raw
		res, err := svc.Config(context.Background(), 1, &cfg, nil)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if res.Round.MarketMode != want {
			t.Fatalf("%q: market mode = %q, want %q", raw, res.Round.MarketMode, want)
		}
	}
}

// Pins the Classic defaults: a round configured from defaults after a reset
// reads back as CLASSIC with preference base 2 and no lean costs.
func TestDefaultRoundIsClassic(t *testing.T) {
	st, svc := newLeanConfigInstructor()
	ctx := context.Background()
	if err := st.ResetGame(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := domain.DefaultRoundConfig()
	if _, err := svc.Config(ctx, 1, &cfg, nil); err != nil {
		t.Fatal(err)
	}
	rd, err := svc.GetRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rd.MarketMode != domain.MarketClassic || rd.PreferenceBase != 2 || rd.ArrivalWindowSeconds != 900 ||
		rd.SetupCostPerBatch != 0 || rd.CarryingCostPerJokeMinute != 0 || rd.WIPLimit != 0 {
		t.Fatalf("default round is not classic: %+v", rd)
	}
}

func TestMergeConfigLeanFields(t *testing.T) {
	base := domain.DefaultRoundConfig()
	mode := "LEAN"
	window, wip := 120, 3
	setup, carry, pref := 1.5, 0.01, 4.0
	got := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{
		MarketMode: &mode, ArrivalWindowSeconds: &window, SetupCostPerBatch: &setup,
		CarryingCostPerJokeMinute: &carry, WIPLimit: &wip, PreferenceBase: &pref,
	})
	if got.MarketMode != domain.MarketLean || got.ArrivalWindowSeconds != 120 || got.SetupCostPerBatch != 1.5 ||
		got.CarryingCostPerJokeMinute != 0.01 || got.WIPLimit != 3 || got.PreferenceBase != 4 {
		t.Fatalf("lean overlay not merged: %+v", got)
	}
	kept := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{})
	if kept != base {
		t.Fatalf("nil overlay changed config: %+v", kept)
	}
}

func TestConfigLocksMarketModeOnceStarted(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	ctx := context.Background()

	// CONFIGURED: the mode may change freely.
	cfg := domain.DefaultRoundConfig()
	if _, err := svc.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	cfg.MarketMode = domain.MarketLean
	if _, err := svc.Config(ctx, 1, &cfg, nil); err != nil {
		t.Fatalf("mode change on CONFIGURED round: %v", err)
	}
	if _, err := svc.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}

	// ACTIVE: re-sending the same mode with other changes is fine.
	cfg.MarketPrice = 2
	if _, err := svc.Config(ctx, 1, &cfg, nil); err != nil {
		t.Fatalf("same mode on ACTIVE round: %v", err)
	}

	// ACTIVE: changing the mode is a conflict and nothing is written.
	cfg.MarketMode = domain.MarketClassic
	cfg.MarketPrice = 3
	if _, err := svc.Config(ctx, 1, &cfg, nil); !domain.IsConflict(err) {
		t.Fatalf("mode change on ACTIVE round: want conflict, got %v", err)
	}
	rd, err := svc.GetRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rd.MarketMode != domain.MarketLean || rd.MarketPrice != 2 {
		t.Fatalf("rejected config was written: %+v", rd)
	}
}

// startedLeanRound configures round 1 as LEAN with non-default lean settings
// and starts it, returning the config it was started with.
func startedLeanRound(t *testing.T, svc *usecase.InstructorService) domain.RoundConfig {
	t.Helper()
	ctx := context.Background()
	cfg := domain.DefaultRoundConfig()
	cfg.MarketMode = domain.MarketLean
	cfg.ArrivalWindowSeconds = 300
	cfg.SetupCostPerBatch = 3
	cfg.CarryingCostPerJokeMinute = 0.02
	cfg.WIPLimit = 2
	cfg.PreferenceBase = 3
	if _, err := svc.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfigLocksLeanSettingsOnceStarted(t *testing.T) {
	for name, mutate := range map[string]func(*domain.RoundConfig){
		"window":   func(c *domain.RoundConfig) { c.ArrivalWindowSeconds = 600 },
		"setup":    func(c *domain.RoundConfig) { c.SetupCostPerBatch = 5 },
		"carrying": func(c *domain.RoundConfig) { c.CarryingCostPerJokeMinute = 0.05 },
		"wip":      func(c *domain.RoundConfig) { c.WIPLimit = 0 },
		"base":     func(c *domain.RoundConfig) { c.PreferenceBase = 2 },
	} {
		_, svc := newLeanConfigInstructor()
		cfg := startedLeanRound(t, svc)
		mutate(&cfg)
		_, err := svc.Config(context.Background(), 1, &cfg, nil)
		var de *domain.DomainError
		if !domain.IsConflict(err) || !errors.As(err, &de) || !strings.Contains(de.Message, "MARKET_MODE_LOCKED") {
			t.Fatalf("%s change on ACTIVE lean round: want MARKET_MODE_LOCKED conflict, got %v", name, err)
		}
		rd, err := svc.GetRound(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if rd.SetupCostPerBatch != 3 || rd.ArrivalWindowSeconds != 300 || rd.CarryingCostPerJokeMinute != 0.02 ||
			rd.WIPLimit != 2 || rd.PreferenceBase != 3 {
			t.Fatalf("%s: rejected config was written: %+v", name, rd)
		}
	}
}

func TestConfigLocksLeanSettingsOnEndedRound(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	cfg := startedLeanRound(t, svc)
	if _, err := svc.EndRound(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	cfg.SetupCostPerBatch = 9
	if _, err := svc.Config(context.Background(), 1, &cfg, nil); !domain.IsConflict(err) {
		t.Fatalf("setup change on ENDED lean round: want conflict, got %v", err)
	}
}

func TestConfigAllowsResendingSameLeanSettingsOnceStarted(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	cfg := startedLeanRound(t, svc)
	// A float that rounds to the stored cents is the same value.
	cfg.CarryingCostPerJokeMinute = 0.020000001
	cfg.MarketPrice = 7
	if _, err := svc.Config(context.Background(), 1, &cfg, nil); err != nil {
		t.Fatalf("resending same lean settings on ACTIVE round: %v", err)
	}
	rd, err := svc.GetRound(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if rd.MarketPrice != 7 {
		t.Fatalf("price change not written: %+v", rd)
	}
}

func TestConfigAllowsLeanSettingChangesBeforeStart(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	ctx := context.Background()
	cfg := domain.DefaultRoundConfig()
	cfg.MarketMode = domain.MarketLean
	if _, err := svc.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	cfg.SetupCostPerBatch = 4
	if _, err := svc.Config(ctx, 1, &cfg, nil); err != nil {
		t.Fatalf("setup change on CONFIGURED round: %v", err)
	}
}

// Classic rounds must keep accepting mid-round price and cost changes.
func TestConfigAllowsClassicPriceChangeOnceStarted(t *testing.T) {
	_, svc := newLeanConfigInstructor()
	ctx := context.Background()
	cfg := domain.DefaultRoundConfig()
	if _, err := svc.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}
	cfg.MarketPrice = 4
	cfg.CostOfPublishing = 1.5
	cfg.CostOfDiscard = 0.5
	cfg.CustomerBudget = 20
	if _, err := svc.Config(ctx, 1, &cfg, nil); err != nil {
		t.Fatalf("price change on ACTIVE classic round: %v", err)
	}
	rd, err := svc.GetRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rd.MarketPrice != 4 || rd.CostOfPublishing != 1.5 || rd.CostOfDiscard != 0.5 || rd.CustomerBudget != 20 {
		t.Fatalf("classic config change not written: %+v", rd)
	}
}
