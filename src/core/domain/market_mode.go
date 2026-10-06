package domain

import (
	"fmt"
	"strings"
)

// MarketMode selects how AI customers buy and how profit is computed.
//
// CLASSIC is the game as shipped: every customer shops the moment a joke is
// classified and swaps for better jokes later. LEAN is the opt-in mode in
// which customers arrive steadily, shop once, and teams pay setup and carrying
// costs. Every Lean rule in the code base is guarded by this value, so a
// CLASSIC round runs the original code paths untouched.
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
