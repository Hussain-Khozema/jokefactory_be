package testutil

import (
	"context"
	"sort"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// now is the store clock, falling back to the wall clock for a zero Store.
func (st *Store) now() time.Time {
	if st.Now == nil {
		return time.Now()
	}
	return st.Now()
}

// leanStats mirrors leanTeamCTE + batchSeriesSQL in the Postgres repo: per
// team, the Lean block for every team with a submitted batch in the round.
func (st *Store) leanStats(rd *domain.Round) map[int64]*ports.LeanTeamStats {
	clockEnd := st.now()
	if rd.EndedAt != nil {
		clockEnd = *rd.EndedAt
	}
	minutes := func(from, to time.Time) float64 {
		d := to.Sub(from).Minutes()
		if d < 0 {
			return 0
		}
		return d
	}

	// first_sale: earliest +1 purchase event per joke.
	firstSale := make(map[int64]time.Time)
	for _, e := range st.PurchaseEvents {
		if e.RoundID != rd.ID || e.Delta <= 0 {
			continue
		}
		if at, ok := firstSale[e.JokeID]; !ok || e.CreatedAt.Before(at) {
			firstSale[e.JokeID] = e.CreatedAt
		}
	}

	type acc struct {
		stats                  *ports.LeanTeamStats
		wipMin, invMin         float64
		submitSum, saleSum     float64
		submitCount, saleCount int
		batches                []*domain.Batch
	}
	byTeam := make(map[int64]*acc)
	for _, b := range st.Batches {
		if b.RoundID != rd.ID || b.SubmittedAt == nil {
			continue
		}
		a := byTeam[b.TeamID]
		if a == nil {
			a = &acc{stats: &ports.LeanTeamStats{BatchSeries: []ports.BatchPoint{}}}
			byTeam[b.TeamID] = a
		}
		a.batches = append(a.batches, b)
		// Every stored batch was created by one submit (one +1 event).
		a.stats.BatchesSubmitted++

		// wip: jokes (batch_size when unsplit) × minutes submitted → processed/clock end.
		jokes := len(b.Jokes)
		if jokes == 0 {
			jokes = rd.BatchSize
		}
		end := clockEnd
		if b.ProcessedAt != nil {
			end = *b.ProcessedAt
			a.submitSum += b.ProcessedAt.Sub(*b.SubmittedAt).Seconds()
			a.submitCount++
		}
		a.wipMin += float64(jokes) * minutes(*b.SubmittedAt, end)

		// inventory: published joke × minutes published → first sale/clock end.
		for _, j := range b.Jokes {
			if j.PublishStatus != domain.PublishPublished || j.PublishedAt == nil {
				continue
			}
			end := clockEnd
			if at, ok := firstSale[j.ID]; ok {
				end = at
				a.saleSum += at.Sub(*j.PublishedAt).Seconds()
				a.saleCount++
			}
			a.invMin += minutes(*j.PublishedAt, end)
		}
	}

	out := make(map[int64]*ports.LeanTeamStats, len(byTeam))
	for teamID, a := range byTeam {
		s := a.stats
		s.SetupCost = float64(s.BatchesSubmitted) * rd.SetupCostPerBatch
		s.CarryingCost = rd.CarryingCostPerJokeMinute * (a.wipMin + a.invMin)
		if a.submitCount > 0 {
			s.AvgSubmitToPublishS = a.submitSum / float64(a.submitCount)
		}
		if a.saleCount > 0 {
			s.AvgPublishToFirstSaleS = a.saleSum / float64(a.saleCount)
		}
		sort.Slice(a.batches, func(i, j int) bool {
			bi, bj := a.batches[i], a.batches[j]
			if !bi.SubmittedAt.Equal(*bj.SubmittedAt) {
				return bi.SubmittedAt.Before(*bj.SubmittedAt)
			}
			return bi.ID < bj.ID
		})
		for i, b := range a.batches {
			p := ports.BatchPoint{Seq: i + 1, SubmittedAt: *b.SubmittedAt, Jokes: len(b.Jokes)}
			for _, j := range b.Jokes {
				if j.PublishStatus == domain.PublishPublished {
					p.Published++
				}
				if _, ok := firstSale[j.ID]; ok {
					p.Sold++
				}
			}
			s.BatchSeries = append(s.BatchSeries, p)
		}
		out[teamID] = s
	}
	return out
}

// emptyLean is the Lean block of a team that has submitted nothing.
func emptyLean() *ports.LeanTeamStats {
	return &ports.LeanTeamStats{BatchSeries: []ports.BatchPoint{}}
}

// leanProfit is the Classic profit minus setup and carrying costs.
func leanProfit(rd *domain.Round, s *domain.TeamRoundState, lean *ports.LeanTeamStats) float64 {
	return rd.MarketPrice*float64(s.PointsEarned) -
		rd.CostOfPublishing*float64(s.PublishedJokes) -
		rd.CostOfDiscard*float64(s.DiscardedJokes) -
		lean.SetupCost - lean.CarryingCost
}

// leanDenseRanks ranks every team of the round by Lean profit, DENSE_RANK style.
func (st *Store) leanDenseRanks(rd *domain.Round, lean map[int64]*ports.LeanTeamStats) map[int64]int {
	var profits []float64
	byTeam := make(map[int64]float64)
	for key, s := range st.TeamState {
		if key[0] != rd.ID {
			continue
		}
		l := lean[key[1]]
		if l == nil {
			l = emptyLean()
		}
		p := leanProfit(rd, s, l)
		byTeam[key[1]] = p
		profits = append(profits, p)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(profits)))
	rankOf := make(map[float64]int)
	for _, p := range profits {
		if _, ok := rankOf[p]; !ok {
			rankOf[p] = len(rankOf) + 1
		}
	}
	out := make(map[int64]int, len(byTeam))
	for teamID, p := range byTeam {
		out[teamID] = rankOf[p]
	}
	return out
}

func (st *Store) GetRoundStatsLean(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	rd, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	lean := st.leanStats(rd)
	ranks := st.leanDenseRanks(rd, lean)
	stats := &ports.RoundStats{RoundID: roundID, Leaderboard: []ports.TeamStats{}}
	for key, state := range st.TeamState {
		if key[0] != roundID {
			continue
		}
		team, err := st.GetTeam(ctx, key[1])
		if err != nil {
			continue
		}
		l := lean[key[1]]
		if l == nil {
			l = emptyLean()
		}
		unsold := state.PublishedJokes - state.PointsEarned
		if unsold < 0 {
			unsold = 0
		}
		stats.Leaderboard = append(stats.Leaderboard, ports.TeamStats{
			Rank: ranks[key[1]], Team: *team, BatchesProcessed: state.BatchesProcessed,
			TotalSales: state.PointsEarned, PublishedJokes: state.PublishedJokes,
			DiscardedJokes: state.DiscardedJokes,
			TotalJokes:     state.PublishedJokes + state.DiscardedJokes,
			UnsoldJokes:    unsold, Profit: leanProfit(rd, state, l), Lean: l,
		})
	}
	sort.Slice(stats.Leaderboard, func(i, j int) bool {
		a, b := stats.Leaderboard[i], stats.Leaderboard[j]
		if a.Profit == b.Profit {
			return a.Team.ID < b.Team.ID
		}
		return a.Profit > b.Profit
	})
	return stats, nil
}

func (st *Store) GetTeamSummaryLean(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	// The Classic summary supplies every count; Lean replaces profit and rank.
	sum, err := st.GetTeamSummary(ctx, roundID, teamID)
	if err != nil {
		return nil, err
	}
	rd := st.Rounds[roundID]
	lean := st.leanStats(rd)
	l := lean[teamID]
	if l == nil {
		l = emptyLean()
	}
	sum.Profit = leanProfit(rd, st.TeamState[[2]int64{roundID, teamID}], l)
	if rank, ok := st.leanDenseRanks(rd, lean)[teamID]; ok {
		sum.Rank = rank
	} else {
		sum.Rank = 1
	}
	sum.Lean = l
	return sum, nil
}
