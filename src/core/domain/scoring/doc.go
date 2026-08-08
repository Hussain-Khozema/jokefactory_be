// Package scoring holds the pure, dependency-free joke-scoring domain logic:
// the 12 judging dimensions, the code-based Length classifier, and the 3-tier
// dim_fit / true_fit computation against an instructor ideal profile.
//
// This package must never import infrastructure (no DB, clock, or rand) so it
// stays fully unit-testable.
package scoring
