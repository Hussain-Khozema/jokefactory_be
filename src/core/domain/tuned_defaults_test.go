package domain

import "testing"

// The defaults the 20-team fee sweep validated
// (docs/superpowers/specs/2026-10-11-tuned-defaults-design.md §2).
func TestTunedDefaults(t *testing.T) {
	if DefaultStorageFeePerJokeMinute != 0 || DefaultBuyThreshold != 8 || DefaultPreferenceBase != 5 ||
		DefaultCustomersPerTeam != 18 {
		t.Fatalf("storage %v, buy threshold %v, preference base %v, customers per team %d; want 0, 8, 5, 18",
			DefaultStorageFeePerJokeMinute, DefaultBuyThreshold, DefaultPreferenceBase, DefaultCustomersPerTeam)
	}
	for _, n := range []int{1, 2} {
		cfg := DefaultRoundConfigFor(n)
		if cfg.StorageFeePerJokeMinute != 0 || cfg.BuyThreshold != 8 || cfg.PreferenceBase != 5 ||
			cfg.CustomersPerTeam != 18 {
			t.Fatalf("round %d defaults %+v; want storage 0, buy threshold 8, preference base 5, 18 customers per team",
				n, cfg)
		}
		// The two costs (listing and hosting); every other fee is 0.
		if cfg.ShippingFeeSmall != 0 || cfg.ShippingFeeStandard != 0 || cfg.ShippingFeeLarge != 0 ||
			cfg.HostingFeePerJokeMinute != 0.05 || cfg.CostOfPublishing != 0.1 || cfg.CostOfDiscard != 0 ||
			cfg.CustomerBudget != 3 || cfg.MarketPrice != 1 || cfg.ArrivalWindowSeconds != 900 || cfg.Jitter != 0.3 {
			t.Fatalf("round %d defaults %+v; the untuned settings changed", n, cfg)
		}
	}
}

func TestConfigFromRoundCarriesCustomersPerTeam(t *testing.T) {
	if got := ConfigFromRound(&Round{CustomersPerTeam: 7}).CustomersPerTeam; got != 7 {
		t.Fatalf("CustomersPerTeam %d; want 7", got)
	}
}
