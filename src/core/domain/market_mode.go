package domain

import (
	"fmt"
	"strings"
)

// MarketMode selects how AI customers buy and how profit is computed.
//
// CLASSIC is the game as first shipped: every customer shops the moment a
// joke is classified and swaps for better jokes later. LEAN, the default on
// this branch, has customers arrive steadily and shop once, and charges teams
// the shipping, storage and hosting fees (lean_fees.go). Every Lean rule in the
// code base is guarded by this value, so a CLASSIC round still runs the
// original code paths untouched.
type MarketMode string

const (
	MarketClassic MarketMode = "CLASSIC"
	MarketLean    MarketMode = "LEAN"
)

// ParseMarketMode accepts either mode case-insensitively; "" means CLASSIC.
func ParseMarketMode(raw string) (MarketMode, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", string(MarketClassic):
		return MarketClassic, nil
	case string(MarketLean):
		return MarketLean, nil
	}
	return "", fmt.Errorf("unknown market_mode %q", raw)
}

func (m MarketMode) IsLean() bool { return m == MarketLean }
