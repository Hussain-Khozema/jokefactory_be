package usecase

import (
	"math"
	"math/rand"
	"strings"
	"time"

	"jokefactory/src/core/ports"
)

// Lean market mode: customers arrive steadily and shop once.
//
// WHY LAZY: there is no scheduler in this backend. Arrival times are stamped
// at round start and served by ServeArrivals (aicustomer.go) whenever the
// round is polled or a batch is classified. Every client polls every 2.5s, so
// customers are served within seconds of their arrival time, and when nothing
// is due the check is one indexed query.

// arrivalTimes spreads n arrivals evenly over window seconds after start:
// offsets (i+1)·window/n for i in 0..n-1, returned in shuffled order so a
// customer's position in the generation loop (and so their threshold jitter)
// is uncorrelated with when they arrive.
func arrivalTimes(start time.Time, windowSeconds, n int, rng *rand.Rand) []time.Time {
	out := make([]time.Time, n)
	for i := 0; i < n; i++ {
		offset := time.Duration(float64(windowSeconds) * float64(i+1) / float64(n) * float64(time.Second))
		out[i] = start.Add(offset)
	}
	rng.Shuffle(n, func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// chooseBasket picks up to n candidates without replacement, each pick
// weighted by base^TrueFit. With base 2, a joke one fit point higher is twice
// as likely to be chosen — better jokes sell more, good jokes still sell.
// Fit is bounded 0..12, so weights stay below 2^12·… and never overflow.
// The input slice is not modified.
//
// A basket never holds the same joke twice. Teams can publish the same text
// more than once (bots that learn re-publish jokes that sold; humans can
// too), and a real customer would not buy two copies of one joke — letting
// them would reward flooding the shelf with duplicates. So after each pick,
// every remaining candidate whose normalised text (normaliseJokeText) equals
// the pick's is dropped from the pool. Candidates with empty text are never
// treated as duplicates of each other. Otherwise the selection is unchanged.
func chooseBasket(cands []ports.CandidateJoke, n int, base float64, rng *rand.Rand) []ports.CandidateJoke {
	pool := append([]ports.CandidateJoke(nil), cands...)
	out := make([]ports.CandidateJoke, 0, n)
	for len(out) < n && len(pool) > 0 {
		total := 0.0
		weights := make([]float64, len(pool))
		for i, c := range pool {
			weights[i] = math.Pow(base, c.TrueFit)
			total += weights[i]
		}
		r := rng.Float64() * total
		idx := len(pool) - 1
		acc := 0.0
		for i, w := range weights {
			acc += w
			if r < acc {
				idx = i
				break
			}
		}
		pick := pool[idx]
		out = append(out, pick)
		pool = append(pool[:idx], pool[idx+1:]...)
		if key := normaliseJokeText(pick.Text); key != "" {
			kept := pool[:0]
			for _, c := range pool {
				if normaliseJokeText(c.Text) != key {
					kept = append(kept, c)
				}
			}
			pool = kept
		}
	}
	return out
}

// normaliseJokeText is the identity chooseBasket uses for "the same joke":
// trimmed, lower-cased, with internal whitespace runs collapsed to one space.
func normaliseJokeText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
