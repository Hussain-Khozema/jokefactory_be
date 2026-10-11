package usecase

import (
	"math"
	"sort"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// MaxLeanChartSamples caps the idle and sell-through series of the lean charts.
const MaxLeanChartSamples = 120

// LeanWaste is a team's cost of waste by kind, in dollars. The JSON keys
// predate the fee names: Setup is the shipping fee, Waiting the storage fee and
// Shelf the hosting fee, so Setup + Waiting + Shelf equals the leaderboard's
// shipping_cost + storage_cost + hosting_cost (= setup_cost + carrying_cost).
type LeanWaste struct {
	Setup     float64 `json:"setup"`
	Waiting   float64 `json:"waiting"`
	Shelf     float64 `json:"shelf"`
	NeverSold float64 `json:"never_sold"`
	Discarded float64 `json:"discarded"`
	Total     float64 `json:"total"`
}

// IdleSample counts idle jokes T seconds after the round started: waiting at
// Marketing and unsold on the shelf. Class averages are means, so fractional.
type IdleSample struct {
	T       int     `json:"t"`
	Waiting float64 `json:"waiting"`
	Shelf   float64 `json:"shelf"`
}

// LeanChartsBatch is one submitted batch. SentS is seconds from the round's
// start; FirstSaleS is seconds from the batch being sent to its first sale,
// nil (JSON null) while none of its jokes has sold.
type LeanChartsBatch struct {
	Seq        int      `json:"seq"`
	SentS      float64  `json:"sent_s"`
	Jokes      int      `json:"jokes"`
	Published  int      `json:"published"`
	Sold       int      `json:"sold"`
	FirstSaleS *float64 `json:"first_sale_s"`
}

// LeanChartsTeam is one team's lean charts.
type LeanChartsTeam struct {
	TeamID   int64             `json:"team_id"`
	TeamName string            `json:"team_name"`
	Waste    LeanWaste         `json:"waste"`
	Idle     []IdleSample      `json:"idle"`
	Batches  []LeanChartsBatch `json:"batches"`
}

// SellThroughSample is the class's pooled sell-through of the batches sent in
// [T, T+sample_seconds): Σ sold ÷ Σ published, nil when nothing was published.
type SellThroughSample struct {
	T     int      `json:"t"`
	Value *float64 `json:"value"`
}

// LeanChartsClassAverage is the anonymous class view: means over every team
// of the round (teams with no batches included) and pooled sell-through.
type LeanChartsClassAverage struct {
	Waste       LeanWaste           `json:"waste"`
	Idle        []IdleSample        `json:"idle"`
	SellThrough []SellThroughSample `json:"sell_through"`
}

// LeanCharts is the body of GET .../lean-charts. Every time is seconds from
// StartedAt; a round that has not started has no samples and no batches.
type LeanCharts struct {
	RoundID       int64                  `json:"round_id"`
	StartedAt     *time.Time             `json:"started_at"`
	ClockEnd      time.Time              `json:"clock_end"`
	SampleSeconds int                    `json:"sample_seconds"`
	WIPLimit      int                    `json:"wip_limit"`
	Teams         []LeanChartsTeam       `json:"teams"`
	ClassAverage  LeanChartsClassAverage `json:"class_average"`
}

// LeanChartSampling returns the sample step in seconds and the number of
// samples for a round with this arrival window whose clock has run for
// elapsed seconds. The step is max(1, round(window / 15)); samples run t = 0,
// step, 2·step, … up to and including the first at or after elapsed. When that
// would exceed MaxLeanChartSamples the step widens so the last sample still
// reaches elapsed: keeping the first 120 would hide the end of a long round.
func LeanChartSampling(windowSeconds int, elapsed float64) (step, count int) {
	step = int(math.Round(float64(windowSeconds) / 15))
	if step < 1 {
		step = 1
	}
	if elapsed < 0 {
		elapsed = 0
	}
	count = int(math.Ceil(elapsed/float64(step))) + 1
	if count > MaxLeanChartSamples {
		step = int(math.Ceil(elapsed / float64(MaxLeanChartSamples-1)))
		count = int(math.Ceil(elapsed/float64(step))) + 1
	}
	return step, count
}

// BuildLeanCharts turns one snapshot of a round's rows into the lean-charts
// body. It is pure; every rule of the lean charts lives here:
//
//	waste      setup = the leaderboard's shipping_cost, waiting = storage fee ×
//	           WIP joke-minutes, shelf = hosting fee × inventory joke-minutes,
//	           never_sold = cost_of_publishing × published jokes never sold,
//	           discarded = cost_of_discard × discarded jokes
//	idle(t)    waiting = Σ jokes of batches with submitted ≤ t < processed;
//	           shelf = published jokes with published ≤ t < first sale
//	jokes      a batch's joke rows, or 5 (domain.StandardShipmentJokes)
//	           while it is unsplit
//	average    mean over every team row; sell-through pooled per sample
func BuildLeanCharts(round *domain.Round, rows *ports.LeanChartRows) LeanCharts {
	out := LeanCharts{
		RoundID:  round.ID,
		ClockEnd: rows.ClockEnd.UTC(),
		WIPLimit: round.WIPLimit,
		Teams:    []LeanChartsTeam{},
		ClassAverage: LeanChartsClassAverage{
			Idle:        []IdleSample{},
			SellThrough: []SellThroughSample{},
		},
	}
	step, count := LeanChartSampling(round.ArrivalWindowSeconds, 0)
	var sec func(time.Time) float64
	if round.StartedAt != nil {
		start := round.StartedAt.UTC()
		out.StartedAt = &start
		sec = func(at time.Time) float64 { return at.Sub(start).Seconds() }
		step, count = LeanChartSampling(round.ArrivalWindowSeconds, sec(rows.ClockEnd))
	} else {
		count = 0
	}
	out.SampleSeconds = step

	jokesIn := func(b ports.LeanChartBatchRow) int {
		if b.JokeRows > 0 {
			return b.JokeRows
		}
		return domain.StandardShipmentJokes
	}
	batchesOf := make(map[int64][]ports.LeanChartBatchRow)
	for _, b := range rows.Batches {
		batchesOf[b.TeamID] = append(batchesOf[b.TeamID], b)
	}
	jokesOf := make(map[int64][]ports.LeanChartJokeRow)
	for _, j := range rows.Jokes {
		jokesOf[j.TeamID] = append(jokesOf[j.TeamID], j)
	}

	teams := append([]ports.LeanChartTeamRow(nil), rows.Teams...)
	sort.Slice(teams, func(i, j int) bool { return teams[i].TeamID < teams[j].TeamID })
	for _, tr := range teams {
		batches := batchesOf[tr.TeamID]
		sort.SliceStable(batches, func(i, j int) bool {
			if !batches[i].SubmittedAt.Equal(batches[j].SubmittedAt) {
				return batches[i].SubmittedAt.Before(batches[j].SubmittedAt)
			}
			return batches[i].BatchID < batches[j].BatchID
		})
		jokes := jokesOf[tr.TeamID]
		team := LeanChartsTeam{
			TeamID:   tr.TeamID,
			TeamName: tr.TeamName,
			Waste:    teamWaste(round, tr, jokes),
			Idle:     []IdleSample{},
			Batches:  []LeanChartsBatch{},
		}
		if sec != nil {
			for k := 0; k < count; k++ {
				team.Idle = append(team.Idle, idleAt(k*step, batches, jokes, sec, jokesIn))
			}
			for i, b := range batches {
				p := LeanChartsBatch{
					Seq: i + 1, SentS: sec(b.SubmittedAt), Jokes: jokesIn(b),
					Published: b.Published, Sold: b.Sold,
				}
				if b.FirstSoldAt != nil {
					v := b.FirstSoldAt.Sub(b.SubmittedAt).Seconds()
					p.FirstSaleS = &v
				}
				team.Batches = append(team.Batches, p)
			}
		}
		out.Teams = append(out.Teams, team)
	}

	out.ClassAverage.Waste = averageWaste(out.Teams)
	for k := 0; k < count; k++ {
		out.ClassAverage.Idle = append(out.ClassAverage.Idle, averageIdle(out.Teams, k, k*step))
	}
	if sec != nil {
		out.ClassAverage.SellThrough = pooledSellThrough(rows.Batches, sec, step, count)
	}
	return out
}

func teamWaste(round *domain.Round, tr ports.LeanChartTeamRow, jokes []ports.LeanChartJokeRow) LeanWaste {
	neverSold := 0
	for _, j := range jokes {
		if j.FirstSoldAt == nil {
			neverSold++
		}
	}
	w := LeanWaste{
		Setup:     tr.ShippingCost,
		Waiting:   round.StorageFeePerJokeMinute * tr.WIPJokeMinutes,
		Shelf:     round.HostingFeePerJokeMinute * tr.InventoryJokeMinutes,
		NeverSold: round.CostOfPublishing * float64(neverSold),
		Discarded: round.CostOfDiscard * float64(tr.DiscardedJokes),
	}
	w.Total = w.Setup + w.Waiting + w.Shelf + w.NeverSold + w.Discarded
	return w
}

// idleAt counts, t seconds after the start, the jokes waiting at Marketing
// (submitted ≤ t < processed) and the published jokes unsold on the shelf
// (published ≤ t < first sale). An interval that never closed stays open.
func idleAt(
	t int,
	batches []ports.LeanChartBatchRow,
	jokes []ports.LeanChartJokeRow,
	sec func(time.Time) float64,
	jokesIn func(ports.LeanChartBatchRow) int,
) IdleSample {
	at := float64(t)
	s := IdleSample{T: t}
	for _, b := range batches {
		if sec(b.SubmittedAt) <= at && (b.ProcessedAt == nil || at < sec(*b.ProcessedAt)) {
			s.Waiting += float64(jokesIn(b))
		}
	}
	for _, j := range jokes {
		if sec(j.PublishedAt) <= at && (j.FirstSoldAt == nil || at < sec(*j.FirstSoldAt)) {
			s.Shelf++
		}
	}
	return s
}

func averageWaste(teams []LeanChartsTeam) LeanWaste {
	var w LeanWaste
	if len(teams) == 0 {
		return w
	}
	for _, t := range teams {
		w.Setup += t.Waste.Setup
		w.Waiting += t.Waste.Waiting
		w.Shelf += t.Waste.Shelf
		w.NeverSold += t.Waste.NeverSold
		w.Discarded += t.Waste.Discarded
	}
	n := float64(len(teams))
	w.Setup /= n
	w.Waiting /= n
	w.Shelf /= n
	w.NeverSold /= n
	w.Discarded /= n
	w.Total = w.Setup + w.Waiting + w.Shelf + w.NeverSold + w.Discarded
	return w
}

func averageIdle(teams []LeanChartsTeam, k, t int) IdleSample {
	s := IdleSample{T: t}
	if len(teams) == 0 {
		return s
	}
	for _, team := range teams {
		s.Waiting += team.Idle[k].Waiting
		s.Shelf += team.Idle[k].Shelf
	}
	n := float64(len(teams))
	s.Waiting /= n
	s.Shelf /= n
	return s
}

// pooledSellThrough pools every team's batches by the sample interval they
// were sent in: Σ sold ÷ Σ published, nil where nothing was published.
func pooledSellThrough(batches []ports.LeanChartBatchRow, sec func(time.Time) float64, step, count int) []SellThroughSample {
	sold := make([]int, count)
	published := make([]int, count)
	for _, b := range batches {
		k := int(math.Floor(sec(b.SubmittedAt) / float64(step)))
		if k < 0 {
			k = 0
		}
		if k >= count {
			k = count - 1
		}
		sold[k] += b.Sold
		published[k] += b.Published
	}
	out := make([]SellThroughSample, 0, count)
	for k := 0; k < count; k++ {
		s := SellThroughSample{T: k * step}
		if published[k] > 0 {
			v := float64(sold[k]) / float64(published[k])
			s.Value = &v
		}
		out = append(out, s)
	}
	return out
}
