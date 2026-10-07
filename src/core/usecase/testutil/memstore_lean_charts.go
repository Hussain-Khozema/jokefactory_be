package testutil

import (
	"context"
	"sort"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// GetLeanChartRows mirrors the Postgres lean-charts queries
// (infra/repo/postgres/lean_charts_repo.go): a row for every
// team_rounds_state row of the round, every submitted batch, and every
// PUBLISHED joke with its first sale. The clock is the store clock, as in
// leanStats, so the two always agree.
func (st *Store) GetLeanChartRows(ctx context.Context, roundID int64) (*ports.LeanChartRows, error) {
	rd, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
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
		if e.RoundID != roundID || e.Delta <= 0 {
			continue
		}
		if at, ok := firstSale[e.JokeID]; !ok || e.CreatedAt.Before(at) {
			firstSale[e.JokeID] = e.CreatedAt
		}
	}

	var batches []*domain.Batch
	for _, b := range st.Batches {
		if b.RoundID == roundID && b.SubmittedAt != nil {
			batches = append(batches, b)
		}
	}
	sort.Slice(batches, func(i, j int) bool {
		bi, bj := batches[i], batches[j]
		if bi.TeamID != bj.TeamID {
			return bi.TeamID < bj.TeamID
		}
		if !bi.SubmittedAt.Equal(*bj.SubmittedAt) {
			return bi.SubmittedAt.Before(*bj.SubmittedAt)
		}
		return bi.ID < bj.ID
	})

	type acc struct {
		submitted  int
		wip, shelf float64
	}
	byTeam := make(map[int64]*acc)
	out := &ports.LeanChartRows{
		ClockEnd: clockEnd.UTC(),
		Teams:    []ports.LeanChartTeamRow{},
		Batches:  []ports.LeanChartBatchRow{},
		Jokes:    []ports.LeanChartJokeRow{},
	}
	for _, b := range batches {
		a := byTeam[b.TeamID]
		if a == nil {
			a = &acc{}
			byTeam[b.TeamID] = a
		}
		a.submitted++
		jokes := len(b.Jokes)
		if jokes == 0 {
			jokes = rd.BatchSize
		}
		row := ports.LeanChartBatchRow{
			BatchID: b.ID, TeamID: b.TeamID, SubmittedAt: b.SubmittedAt.UTC(), JokeRows: len(b.Jokes),
		}
		end := clockEnd
		if b.ProcessedAt != nil {
			end = *b.ProcessedAt
			p := b.ProcessedAt.UTC()
			row.ProcessedAt = &p
		}
		a.wip += float64(jokes) * minutes(*b.SubmittedAt, end)

		for _, j := range b.Jokes {
			sold, hasSale := firstSale[j.ID]
			if hasSale {
				row.Sold++
				if row.FirstSoldAt == nil || sold.Before(*row.FirstSoldAt) {
					s := sold.UTC()
					row.FirstSoldAt = &s
				}
			}
			if j.PublishStatus != domain.PublishPublished {
				continue
			}
			row.Published++
			if j.PublishedAt == nil {
				continue
			}
			jokeRow := ports.LeanChartJokeRow{TeamID: b.TeamID, PublishedAt: j.PublishedAt.UTC()}
			shelfEnd := clockEnd
			if hasSale {
				shelfEnd = sold
				s := sold.UTC()
				jokeRow.FirstSoldAt = &s
			}
			a.shelf += minutes(*j.PublishedAt, shelfEnd)
			out.Jokes = append(out.Jokes, jokeRow)
		}
		out.Batches = append(out.Batches, row)
	}

	for key, state := range st.TeamState {
		if key[0] != roundID {
			continue
		}
		team, err := st.GetTeam(ctx, key[1])
		if err != nil {
			continue
		}
		row := ports.LeanChartTeamRow{TeamID: team.ID, TeamName: team.Name, DiscardedJokes: state.DiscardedJokes}
		if a := byTeam[team.ID]; a != nil {
			row.BatchesSubmitted, row.WIPJokeMinutes, row.InventoryJokeMinutes = a.submitted, a.wip, a.shelf
		}
		out.Teams = append(out.Teams, row)
	}
	sort.Slice(out.Teams, func(i, j int) bool { return out.Teams[i].TeamID < out.Teams[j].TeamID })
	sort.SliceStable(out.Jokes, func(i, j int) bool {
		ji, jj := out.Jokes[i], out.Jokes[j]
		if ji.TeamID != jj.TeamID {
			return ji.TeamID < jj.TeamID
		}
		return ji.PublishedAt.Before(jj.PublishedAt)
	})
	return out, nil
}
