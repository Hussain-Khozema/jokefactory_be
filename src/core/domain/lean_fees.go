package domain

// StandardShipmentJokes is the standard shipment: a batch Marketing has not
// split yet counts as this many jokes for both the storage fee and the
// shipping fee, whatever the round's batch_size.
const StandardShipmentJokes = 5

// ShippingFeeFor is the shipping fee of one sent batch with this many jokes:
// small for 1 to 4, standard for exactly 5, large for 6 or more. A count of 0
// or less is an unsplit batch, which ships as the standard 5.
func ShippingFeeFor(jokes int, small, standard, large float64) float64 {
	if jokes <= 0 {
		jokes = StandardShipmentJokes
	}
	switch {
	case jokes < StandardShipmentJokes:
		return small
	case jokes == StandardShipmentJokes:
		return standard
	default:
		return large
	}
}

// DefaultRoundConfigFor is DefaultRoundConfig with the batch size of that
// round: exactly DefaultInstructorBatchSize in Round 1, up to
// DefaultRound2BatchSize from Round 2 on.
func DefaultRoundConfigFor(roundNumber int) RoundConfig {
	cfg := DefaultRoundConfig()
	if roundNumber >= 2 {
		cfg.BatchSize = DefaultRound2BatchSize
	}
	return cfg
}
