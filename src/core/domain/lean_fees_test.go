package domain

import "testing"

// One Lean rule set for both rounds, with two costs: listing and hosting
// (docs/superpowers/specs/2026-11-10-two-costs-feedback-bots-design.md §2).
func TestDefaultRoundConfigIsTheLeanRuleSet(t *testing.T) {
	cfg := DefaultRoundConfig()
	if cfg.MarketMode != MarketLean {
		t.Fatalf("default mode %q; want LEAN", cfg.MarketMode)
	}
	if cfg.ShippingFeeSmall != 0 || cfg.ShippingFeeStandard != 0 || cfg.ShippingFeeLarge != 0 {
		t.Fatalf("shipping fees %v / %v / %v; want 0 / 0 / 0",
			cfg.ShippingFeeSmall, cfg.ShippingFeeStandard, cfg.ShippingFeeLarge)
	}
	if cfg.StorageFeePerJokeMinute != 0 || cfg.CostOfDiscard != 0 {
		t.Fatalf("storage %v, discard %v; want 0, 0", cfg.StorageFeePerJokeMinute, cfg.CostOfDiscard)
	}
	if cfg.CostOfPublishing != 0.10 || cfg.HostingFeePerJokeMinute != 0.05 {
		t.Fatalf("listing %v, hosting %v; want 0.10, 0.05", cfg.CostOfPublishing, cfg.HostingFeePerJokeMinute)
	}
	if cfg.WIPLimit != 0 || cfg.PreferenceBase != 5 || cfg.CustomerCount != 150 || cfg.ArrivalWindowSeconds != 900 {
		t.Fatalf("wip %d, preference base %v, customers %d, window %d; want 0, 5, 150, 900",
			cfg.WIPLimit, cfg.PreferenceBase, cfg.CustomerCount, cfg.ArrivalWindowSeconds)
	}
	if cfg.BatchSize != 5 {
		t.Fatalf("round 1 batch size %d; want 5", cfg.BatchSize)
	}
}

func TestDefaultRoundConfigForGivesRound2UpToTenJokes(t *testing.T) {
	if got := DefaultRoundConfigFor(1).BatchSize; got != 5 {
		t.Fatalf("round 1 batch size %d; want 5", got)
	}
	for _, n := range []int{2, 3} {
		got := DefaultRoundConfigFor(n)
		if got.BatchSize != 10 {
			t.Fatalf("round %d batch size %d; want 10", n, got.BatchSize)
		}
		got.BatchSize = 5
		if got != DefaultRoundConfig() {
			t.Fatalf("round %d differs from the defaults beyond batch size: %+v", n, got)
		}
	}
}

func TestShippingFeeForTiers(t *testing.T) {
	for _, tc := range []struct {
		jokes int
		want  float64
	}{
		{0, 2}, // unsplit: the standard shipment
		{-1, 2},
		{1, 1}, {4, 1},
		{5, 2},
		{6, 3}, {10, 3}, {12, 3},
	} {
		if got := ShippingFeeFor(tc.jokes, 1, 2, 3); got != tc.want {
			t.Errorf("ShippingFeeFor(%d) = %v; want %v", tc.jokes, got, tc.want)
		}
	}
}
