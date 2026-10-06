package domain

import "testing"

func TestParseMarketMode(t *testing.T) {
	cases := map[string]struct {
		want MarketMode
		ok   bool
	}{
		"CLASSIC": {MarketClassic, true},
		"LEAN":    {MarketLean, true},
		"lean":    {MarketLean, true},
		"":        {MarketClassic, true},
		"bogus":   {"", false},
	}
	for in, c := range cases {
		got, err := ParseMarketMode(in)
		if c.ok && (err != nil || got != c.want) {
			t.Fatalf("ParseMarketMode(%q) = %q, %v; want %q", in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Fatalf("ParseMarketMode(%q) accepted", in)
		}
	}
}

func TestDefaultRoundConfigIsClassic(t *testing.T) {
	cfg := DefaultRoundConfig()
	if cfg.MarketMode != MarketClassic {
		t.Fatalf("default mode %q", cfg.MarketMode)
	}
	if cfg.ArrivalWindowSeconds != 900 || cfg.PreferenceBase != 2 || cfg.SetupCostPerBatch != 0 ||
		cfg.CarryingCostPerJokeMinute != 0 || cfg.WIPLimit != 0 {
		t.Fatalf("lean defaults wrong: %+v", cfg)
	}
}
