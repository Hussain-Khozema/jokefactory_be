//nolint:gocognit,gocritic,gocyclo // test fake mirrors production repository behavior
package testutil

import (
	"context"
	"sort"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

type Store struct {
	Users        map[int64]*domain.User
	ByName       map[string]int64
	Teams        []domain.Team
	Rounds       map[int64]*domain.Round
	Ideals       map[int64]domain.IdealProfile
	TeamState    map[[2]int64]*domain.TeamRoundState
	Batches      map[int64]*domain.Batch
	ClassJobs    map[int64]*domain.ClassificationJob
	DimValues    map[int64]map[domain.Dimension]string
	DimFits      map[int64]map[domain.Dimension]float64
	JokeFits     map[int64]*domain.JokeFit
	AICustomers  map[int64]*domain.AICustomer // keyed by ai_customer_id
	Purchases    map[int64]*domain.Purchase   // keyed by purchase_id
	NextUser     int64
	NextTeam     int64
	NextBatch    int64
	NextJoke     int64
	NextAICust   int64
	NextPurchase int64
}

func NewStore() *Store {
	return &Store{
		Users:        make(map[int64]*domain.User),
		ByName:       make(map[string]int64),
		Rounds:       make(map[int64]*domain.Round),
		Ideals:       make(map[int64]domain.IdealProfile),
		TeamState:    make(map[[2]int64]*domain.TeamRoundState),
		Batches:      make(map[int64]*domain.Batch),
		ClassJobs:    make(map[int64]*domain.ClassificationJob),
		DimValues:    make(map[int64]map[domain.Dimension]string),
		DimFits:      make(map[int64]map[domain.Dimension]float64),
		JokeFits:     make(map[int64]*domain.JokeFit),
		AICustomers:  make(map[int64]*domain.AICustomer),
		Purchases:    make(map[int64]*domain.Purchase),
		NextUser:     1,
		NextTeam:     1,
		NextBatch:    1,
		NextJoke:     1,
		NextAICust:   1,
		NextPurchase: 1,
	}
}

func (st *Store) Health(context.Context) error { return nil }

func (st *Store) CreateUser(_ context.Context, displayName string) (*domain.User, error) {
	id := st.NextUser
	st.NextUser++
	now := time.Now().UTC()
	u := &domain.User{
		ID: id, DisplayName: displayName, Status: domain.ParticipantWaiting,
		JoinedAt: now, CreatedAt: now,
	}
	st.Users[id] = u
	st.ByName[displayName] = id
	return cloneUser(u), nil
}

func (st *Store) GetUserByDisplayName(_ context.Context, displayName string) (*domain.User, error) {
	id, ok := st.ByName[displayName]
	if !ok {
		return nil, domain.NewNotFoundError("user")
	}
	return cloneUser(st.Users[id]), nil
}

func (st *Store) GetUserByID(_ context.Context, userID int64) (*domain.User, error) {
	u, ok := st.Users[userID]
	if !ok {
		return nil, domain.NewNotFoundError("user")
	}
	return cloneUser(u), nil
}

func (st *Store) UpdateUserAssignment(_ context.Context, userID int64, role *domain.Role, teamID *int64) error {
	u, ok := st.Users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Role = role
	u.TeamID = teamID
	return nil
}

func (st *Store) UpdateUserStatus(_ context.Context, userID int64, status domain.ParticipantStatus) error {
	u, ok := st.Users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Status = status
	return nil
}

func (st *Store) PatchUserInRound(_ context.Context, _, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) error {
	u, ok := st.Users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Status = status
	u.Role = role
	u.TeamID = teamID
	if status == domain.ParticipantAssigned {
		now := time.Now().UTC()
		u.AssignedAt = &now
	} else {
		u.AssignedAt = nil
	}
	return nil
}

func (st *Store) MarkUserAssigned(_ context.Context, userID int64) error {
	u, ok := st.Users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Status = domain.ParticipantAssigned
	now := time.Now().UTC()
	u.AssignedAt = &now
	return nil
}

func (st *Store) ListUsersByStatus(_ context.Context, status domain.ParticipantStatus) ([]domain.User, error) {
	var out []domain.User
	for _, u := range st.Users {
		if u.Status == status && (u.Role == nil || *u.Role != domain.RoleInstructor) {
			out = append(out, *cloneUser(u))
		}
	}
	return out, nil
}

func (st *Store) ListTeamMembers(_ context.Context, teamID int64) ([]ports.TeamMember, error) {
	var out []ports.TeamMember
	for _, u := range st.Users {
		if u.TeamID != nil && *u.TeamID == teamID && u.Role != nil {
			out = append(out, ports.TeamMember{UserID: u.ID, DisplayName: u.DisplayName, Role: *u.Role})
		}
	}
	return out, nil
}

func (st *Store) DeleteUser(_ context.Context, userID int64) error {
	u, ok := st.Users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	if u.Role != nil && *u.Role == domain.RoleInstructor {
		return domain.NewConflictError("cannot delete instructor user")
	}
	delete(st.ByName, u.DisplayName)
	delete(st.Users, userID)
	return nil
}

func (st *Store) EnsureTeamCount(_ context.Context, teamCount int) ([]domain.Team, error) {
	for len(st.Teams) < teamCount {
		id := st.NextTeam
		st.NextTeam++
		st.Teams = append(st.Teams, domain.Team{ID: id, Name: "Team " + itoa(id), CreatedAt: time.Now().UTC()})
	}
	out := make([]domain.Team, len(st.Teams))
	copy(out, st.Teams)
	return out, nil
}

func (st *Store) GetTeam(_ context.Context, teamID int64) (*domain.Team, error) {
	for i := range st.Teams {
		if st.Teams[i].ID == teamID {
			t := st.Teams[i]
			return &t, nil
		}
	}
	return nil, domain.NewNotFoundError("team")
}

func (st *Store) GetActiveRound(_ context.Context) (*domain.Round, error) {
	for _, r := range st.Rounds {
		if r.Status == domain.RoundActive {
			return cloneRound(r), nil
		}
	}
	// Intentionally nil,nil — mirrors postgres.GetActiveRound when none active.
	return nil, nil //nolint:nilnil
}

func (st *Store) GetRoundByID(_ context.Context, roundID int64) (*domain.Round, error) {
	r, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	return cloneRound(r), nil
}

func (st *Store) GetLatestRound(_ context.Context) (*domain.Round, error) {
	var best *domain.Round
	for _, r := range st.Rounds {
		if best == nil || r.ID > best.ID {
			best = r
		}
	}
	if best == nil {
		// Intentionally nil,nil — mirrors postgres.GetLatestRound when empty.
		return nil, nil //nolint:nilnil
	}
	return cloneRound(best), nil
}

func (st *Store) ListRounds(_ context.Context) ([]domain.Round, error) {
	out := make([]domain.Round, 0, len(st.Rounds))
	for _, r := range st.Rounds {
		out = append(out, *cloneRound(r))
	}
	return out, nil
}

func (st *Store) UpdateRoundConfig(_ context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error) {
	r, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	applyConfig(r, cfg)
	return cloneRound(r), nil
}

func (st *Store) InsertRoundConfig(_ context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error) {
	if r, ok := st.Rounds[roundID]; ok {
		applyConfig(r, cfg)
		return cloneRound(r), nil
	}
	r := &domain.Round{
		ID: roundID, RoundNumber: int(roundID), Status: domain.RoundConfigured, CreatedAt: time.Now().UTC(),
	}
	applyConfig(r, cfg)
	st.Rounds[roundID] = r
	return cloneRound(r), nil
}

func (st *Store) UpsertIdealProfile(_ context.Context, roundID int64, profile domain.IdealProfile) error {
	cp := make(domain.IdealProfile, len(profile))
	for k, v := range profile {
		cp[k] = v
	}
	st.Ideals[roundID] = cp
	return nil
}

func (st *Store) GetIdealProfile(_ context.Context, roundID int64) (domain.IdealProfile, error) {
	p := st.Ideals[roundID]
	if p == nil {
		return domain.IdealProfile{}, nil
	}
	cp := make(domain.IdealProfile, len(p))
	for k, v := range p {
		cp[k] = v
	}
	return cp, nil
}

func (st *Store) StartRound(_ context.Context, roundID int64) (*domain.Round, error) {
	r, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	r.Status = domain.RoundActive
	now := time.Now().UTC()
	r.StartedAt = &now
	r.EndedAt = nil
	for _, t := range st.Teams {
		st.ensureTeamState(roundID, t.ID)
	}
	return cloneRound(r), nil
}

func (st *Store) EndRound(_ context.Context, roundID int64) (*domain.Round, error) {
	r, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	r.Status = domain.RoundEnded
	now := time.Now().UTC()
	r.EndedAt = &now
	return cloneRound(r), nil
}

func (st *Store) SetRoundPopupState(_ context.Context, roundID int64, isActive bool) (*domain.Round, error) {
	r, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	r.IsPoppedActive = isActive
	return cloneRound(r), nil
}

func (st *Store) EnsureTeamRoundState(_ context.Context, roundID, teamID int64) error {
	st.ensureTeamState(roundID, teamID)
	return nil
}

func (st *Store) ensureTeamState(roundID, teamID int64) *domain.TeamRoundState {
	key := [2]int64{roundID, teamID}
	if s, ok := st.TeamState[key]; ok {
		return s
	}
	now := time.Now().UTC()
	s := &domain.TeamRoundState{RoundID: roundID, TeamID: teamID, CreatedAt: now, UpdatedAt: now}
	st.TeamState[key] = s
	return s
}

func (st *Store) ResetGame(context.Context) error {
	*st = *NewStore()
	return nil
}

func (st *Store) CreateBatch(_ context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error) {
	id := st.NextBatch
	st.NextBatch++
	now := time.Now().UTC()
	b := &domain.Batch{
		ID: id, RoundID: roundID, TeamID: teamID, Status: domain.BatchSubmitted,
		SubmittedAt: &now, CreatedAt: now,
	}
	for _, text := range jokes {
		jid := st.NextJoke
		st.NextJoke++
		b.Jokes = append(b.Jokes, domain.Joke{
			ID: jid, BatchID: id, Text: text, PublishStatus: domain.PublishPending, CreatedAt: now,
		})
	}
	st.Batches[id] = b
	state := st.ensureTeamState(roundID, teamID)
	state.BatchesCreated++
	state.UpdatedAt = now
	return cloneBatch(b), nil
}

func (st *Store) ListBatchesByTeam(_ context.Context, roundID, teamID int64) ([]domain.Batch, error) {
	var out []domain.Batch
	for _, b := range st.Batches {
		if b.RoundID == roundID && b.TeamID == teamID {
			out = append(out, *cloneBatch(b))
		}
	}
	return out, nil
}

func (st *Store) GetBatchWithJokes(_ context.Context, batchID int64) (*ports.BatchWithJokes, error) {
	b, ok := st.Batches[batchID]
	if !ok {
		return nil, domain.NewNotFoundError("batch")
	}
	cp := cloneBatch(b)
	return &ports.BatchWithJokes{Batch: *cp, Jokes: cp.Jokes}, nil
}

func (st *Store) CountSubmittedBatchesForTeam(_ context.Context, roundID, teamID int64) (int, error) {
	n := 0
	for _, b := range st.Batches {
		if b.RoundID == roundID && b.TeamID == teamID && b.Status == domain.BatchSubmitted {
			n++
		}
	}
	return n, nil
}

func (st *Store) ClaimNextBatch(_ context.Context, roundID, teamID, marketerID int64) (*ports.BatchWithJokes, error) {
	var held *domain.Batch
	var next *domain.Batch
	for _, b := range st.Batches {
		if b.RoundID != roundID || b.TeamID != teamID || b.Status != domain.BatchSubmitted {
			continue
		}
		if b.LockedBy != nil && *b.LockedBy == marketerID {
			if held == nil || b.ID < held.ID {
				held = b
			}
			continue
		}
		if b.LockedBy == nil {
			if next == nil || b.ID < next.ID {
				next = b
			}
		}
	}
	target := held
	if target == nil {
		target = next
	}
	if target == nil {
		return nil, nil //nolint:nilnil // empty queue mirrors postgres
	}
	now := time.Now().UTC()
	target.LockedBy = &marketerID
	target.LockedAt = &now
	cp := cloneBatch(target)
	return &ports.BatchWithJokes{Batch: *cp, Jokes: cp.Jokes}, nil
}

func (st *Store) PublishBatch(
	_ context.Context,
	batchID, marketerID, teamID int64,
	decisions []ports.JokePublishDecision,
) (*ports.PublishResult, error) {
	b, ok := st.Batches[batchID]
	if !ok {
		return nil, domain.NewNotFoundError("batch")
	}
	if b.TeamID != teamID {
		return nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	if b.Status == domain.BatchProcessed {
		return nil, domain.NewConflictError("BATCH_ALREADY_PROCESSED")
	}
	if b.Status != domain.BatchSubmitted {
		return nil, domain.NewConflictError("batch not submitted")
	}
	if b.LockedBy == nil || *b.LockedBy != marketerID {
		return nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	if len(decisions) != len(b.Jokes) {
		return nil, domain.NewValidationError("jokes", "expected decisions for every joke")
	}

	byID := make(map[int64]*domain.Joke, len(b.Jokes))
	for i := range b.Jokes {
		byID[b.Jokes[i].ID] = &b.Jokes[i]
	}
	published := make([]int64, 0)
	discarded := make([]int64, 0)
	seen := make(map[int64]struct{})
	now := time.Now().UTC()

	for _, d := range decisions {
		j, ok := byID[d.JokeID]
		if !ok {
			return nil, domain.NewValidationError("jokes", "joke not in batch")
		}
		if _, dup := seen[d.JokeID]; dup {
			return nil, domain.NewValidationError("jokes", "duplicate joke_id")
		}
		seen[d.JokeID] = struct{}{}
		title := d.Title
		if d.IsPublished {
			if title == "" {
				return nil, domain.NewValidationError("joke_title", "title required for published jokes")
			}
			j.Title = &title
			j.PublishStatus = domain.PublishPublished
			j.PublishedAt = &now
			published = append(published, d.JokeID)
		} else {
			if title != "" {
				j.Title = &title
			}
			j.PublishStatus = domain.PublishDiscarded
			j.PublishedAt = nil
			discarded = append(discarded, d.JokeID)
		}
	}
	if len(published) == 0 {
		return nil, domain.NewValidationError("jokes", "NO_JOKE_PUBLISHED")
	}

	b.Status = domain.BatchProcessed
	b.ProcessedAt = &now
	b.LockedBy = nil
	b.LockedAt = nil

	state := st.ensureTeamState(b.RoundID, b.TeamID)
	state.BatchesProcessed++
	state.PublishedJokes += len(published)
	state.DiscardedJokes += len(discarded)
	state.UpdatedAt = now

	if _, ok := st.ClassJobs[batchID]; !ok {
		st.ClassJobs[batchID] = &domain.ClassificationJob{
			BatchID:   batchID,
			RoundID:   b.RoundID,
			Status:    domain.ClassificationPending,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	cp := cloneBatch(b)
	return &ports.PublishResult{
		Batch:        *cp,
		PublishedIDs: published,
		DiscardedIDs: discarded,
	}, nil
}

func (st *Store) EnsureClassificationJob(_ context.Context, batchID, roundID int64) error {
	if _, ok := st.ClassJobs[batchID]; ok {
		return nil
	}
	now := time.Now().UTC()
	st.ClassJobs[batchID] = &domain.ClassificationJob{
		BatchID: batchID, RoundID: roundID, Status: domain.ClassificationPending,
		CreatedAt: now, UpdatedAt: now,
	}
	return nil
}

func (st *Store) ClaimClassificationJob(_ context.Context, batchID int64) (*domain.ClassificationJob, error) {
	job, ok := st.ClassJobs[batchID]
	if !ok {
		return nil, nil //nolint:nilnil
	}
	if job.Attempts >= ports.MaxClassificationAttempts {
		return nil, nil //nolint:nilnil
	}
	switch job.Status {
	case domain.ClassificationDone:
		return nil, nil //nolint:nilnil
	case domain.ClassificationProcessing:
		if time.Since(job.UpdatedAt) < ports.StaleClassificationAfter {
			return nil, nil //nolint:nilnil
		}
	case domain.ClassificationPending, domain.ClassificationFailed:
		// claimable
	default:
		return nil, nil //nolint:nilnil
	}
	job.Status = domain.ClassificationProcessing
	job.Attempts++
	job.LastError = nil
	job.UpdatedAt = time.Now().UTC()
	cp := *job
	return &cp, nil
}

func (st *Store) MarkClassificationDone(_ context.Context, batchID int64, model string) error {
	job, ok := st.ClassJobs[batchID]
	if !ok {
		return domain.NewNotFoundError("classification_job")
	}
	now := time.Now().UTC()
	job.Status = domain.ClassificationDone
	job.Model = &model
	job.LastError = nil
	job.ClassifiedAt = &now
	job.UpdatedAt = now
	return nil
}

func (st *Store) MarkClassificationFailed(_ context.Context, batchID int64, errMsg string) error {
	job, ok := st.ClassJobs[batchID]
	if !ok {
		return domain.NewNotFoundError("classification_job")
	}
	job.LastError = &errMsg
	job.UpdatedAt = time.Now().UTC()
	if job.Attempts >= ports.MaxClassificationAttempts {
		job.Status = domain.ClassificationFailed
	} else {
		job.Status = domain.ClassificationPending
	}
	return nil
}

func (st *Store) PersistJokeFits(_ context.Context, fits []ports.JokeFitMaterialization) error {
	now := time.Now().UTC()
	for _, fit := range fits {
		cats := make(map[domain.Dimension]string, len(fit.Categories))
		for k, v := range fit.Categories {
			cats[k] = v
		}
		scores := make(map[domain.Dimension]float64, len(fit.DimFits))
		for k, v := range fit.DimFits {
			scores[k] = v
		}
		st.DimValues[fit.JokeID] = cats
		st.DimFits[fit.JokeID] = scores
		st.JokeFits[fit.JokeID] = &domain.JokeFit{
			JokeID: fit.JokeID, RoundID: fit.RoundID, TrueFit: fit.TrueFit, ComputedAt: now,
		}
	}
	return nil
}

func (st *Store) GetJokeFit(_ context.Context, jokeID int64) (*domain.JokeFit, error) {
	f, ok := st.JokeFits[jokeID]
	if !ok {
		return nil, domain.NewNotFoundError("joke_fit")
	}
	cp := *f
	return &cp, nil
}

func (st *Store) ListJokeDimFits(_ context.Context, jokeID int64) ([]domain.JokeDimFit, error) {
	scores, ok := st.DimFits[jokeID]
	if !ok {
		return nil, nil
	}
	out := make([]domain.JokeDimFit, 0, len(scores))
	for dim, score := range scores {
		out = append(out, domain.JokeDimFit{JokeID: jokeID, Dimension: dim, DimFit: score})
	}
	return out, nil
}

func (st *Store) ListJokeDimensionValues(_ context.Context, jokeID int64) ([]domain.JokeDimensionValue, error) {
	cats, ok := st.DimValues[jokeID]
	if !ok {
		return nil, nil
	}
	out := make([]domain.JokeDimensionValue, 0, len(cats))
	for dim, cat := range cats {
		out = append(out, domain.JokeDimensionValue{JokeID: jokeID, Dimension: dim, Category: cat})
	}
	return out, nil
}

func (st *Store) ListOrphanClassificationBatchIDs(_ context.Context, limit int) ([]int64, error) {
	var ids []int64
	for _, b := range st.Batches {
		if b.Status != domain.BatchProcessed {
			continue
		}
		hasPublished := false
		for _, j := range b.Jokes {
			if j.PublishStatus == domain.PublishPublished {
				hasPublished = true
				break
			}
		}
		if !hasPublished {
			continue
		}
		job, ok := st.ClassJobs[b.ID]
		if !ok {
			ids = append(ids, b.ID)
			continue
		}
		if job.Attempts >= ports.MaxClassificationAttempts {
			continue
		}
		switch job.Status {
		case domain.ClassificationDone:
			continue
		case domain.ClassificationPending, domain.ClassificationFailed:
			ids = append(ids, b.ID)
		case domain.ClassificationProcessing:
			if time.Since(job.UpdatedAt) >= ports.StaleClassificationAfter {
				ids = append(ids, b.ID)
			}
		}
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (st *Store) ReplaceAICustomers(_ context.Context, roundID int64, customers []domain.AICustomer) error {
	for id, c := range st.AICustomers {
		if c.RoundID == roundID {
			delete(st.AICustomers, id)
		}
	}
	now := time.Now().UTC()
	for _, c := range customers {
		id := st.NextAICust
		st.NextAICust++
		cp := c
		cp.ID = id
		cp.RoundID = roundID
		cp.CreatedAt = now
		st.AICustomers[id] = &cp
	}
	return nil
}

func (st *Store) ListAICustomers(_ context.Context, roundID int64) ([]domain.AICustomer, error) {
	var out []domain.AICustomer
	for _, c := range st.AICustomers {
		if c.RoundID == roundID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (st *Store) ListCandidateJokes(_ context.Context, jokeIDs []int64) ([]ports.CandidateJoke, error) {
	want := make(map[int64]struct{}, len(jokeIDs))
	for _, id := range jokeIDs {
		want[id] = struct{}{}
	}
	var out []ports.CandidateJoke
	for _, b := range st.Batches {
		for _, j := range b.Jokes {
			if _, ok := want[j.ID]; !ok {
				continue
			}
			if j.PublishStatus != domain.PublishPublished {
				continue
			}
			fit, ok := st.JokeFits[j.ID]
			if !ok {
				continue
			}
			out = append(out, ports.CandidateJoke{
				JokeID: j.ID, TeamID: b.TeamID, RoundID: b.RoundID, TrueFit: fit.TrueFit,
			})
		}
	}
	return out, nil
}

func (st *Store) ListHoldings(_ context.Context, roundID, aiCustomerID int64) ([]ports.HeldJoke, error) {
	var out []ports.HeldJoke
	for _, p := range st.Purchases {
		if p.RoundID != roundID || p.AICustomerID != aiCustomerID {
			continue
		}
		tf := 0.0
		if f, ok := st.JokeFits[p.JokeID]; ok {
			tf = f.TrueFit
		}
		out = append(out, ports.HeldJoke{
			JokeID: p.JokeID, TeamID: p.TeamID, TrueFit: tf, Price: p.Price,
		})
	}
	return out, nil
}

func (st *Store) BuyJoke(_ context.Context, roundID, aiCustomerID, jokeID, teamID int64, price float64) error {
	c, ok := st.AICustomers[aiCustomerID]
	if !ok || c.RoundID != roundID {
		return domain.NewNotFoundError("ai_customer")
	}
	for _, p := range st.Purchases {
		if p.RoundID == roundID && p.AICustomerID == aiCustomerID && p.JokeID == jokeID {
			return nil
		}
	}
	if c.RemainingBudget < price {
		return domain.NewConflictError("insufficient budget")
	}
	c.RemainingBudget -= price
	id := st.NextPurchase
	st.NextPurchase++
	now := time.Now().UTC()
	st.Purchases[id] = &domain.Purchase{
		ID: id, RoundID: roundID, AICustomerID: aiCustomerID,
		JokeID: jokeID, TeamID: teamID, Price: price, CreatedAt: now,
	}
	state := st.ensureTeamState(roundID, teamID)
	state.PointsEarned++
	state.UpdatedAt = now
	return nil
}

func (st *Store) SwapJoke(
	_ context.Context,
	roundID, aiCustomerID, buyJokeID, buyTeamID, returnJokeID, returnTeamID int64,
	price float64,
) error {
	if _, ok := st.AICustomers[aiCustomerID]; !ok {
		return domain.NewNotFoundError("ai_customer")
	}
	var returned bool
	for id, p := range st.Purchases {
		if p.RoundID == roundID && p.AICustomerID == aiCustomerID && p.JokeID == returnJokeID {
			delete(st.Purchases, id)
			returned = true
			break
		}
	}
	if !returned {
		return domain.NewConflictError("held joke not found for swap")
	}
	stRet := st.ensureTeamState(roundID, returnTeamID)
	if stRet.PointsEarned > 0 {
		stRet.PointsEarned--
	}
	for _, p := range st.Purchases {
		if p.RoundID == roundID && p.AICustomerID == aiCustomerID && p.JokeID == buyJokeID {
			return domain.NewConflictError("already holds buy target")
		}
	}
	id := st.NextPurchase
	st.NextPurchase++
	now := time.Now().UTC()
	st.Purchases[id] = &domain.Purchase{
		ID: id, RoundID: roundID, AICustomerID: aiCustomerID,
		JokeID: buyJokeID, TeamID: buyTeamID, Price: price, CreatedAt: now,
	}
	stBuy := st.ensureTeamState(roundID, buyTeamID)
	stBuy.PointsEarned++
	stBuy.UpdatedAt = now
	stRet.UpdatedAt = now
	return nil
}

func (st *Store) ListMarket(_ context.Context, roundID int64) ([]ports.MarketJoke, error) {
	sold := make(map[int64]int)
	for _, p := range st.Purchases {
		if p.RoundID == roundID {
			sold[p.JokeID]++
		}
	}
	teamName := func(id int64) string {
		for _, t := range st.Teams {
			if t.ID == id {
				return t.Name
			}
		}
		return ""
	}
	var out []ports.MarketJoke
	for _, b := range st.Batches {
		if b.RoundID != roundID {
			continue
		}
		for _, j := range b.Jokes {
			if j.PublishStatus != domain.PublishPublished {
				continue
			}
			out = append(out, ports.MarketJoke{
				JokeID: j.ID, JokeText: j.Text, JokeTitle: j.Title,
				TeamID: b.TeamID, TeamName: teamName(b.TeamID),
				SoldCount: sold[j.ID], PublishedAt: j.PublishedAt,
			})
		}
	}
	return out, nil
}

func (st *Store) GetTeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	rd, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	team, err := st.GetTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	teamState := st.ensureTeamState(roundID, teamID)
	profit := rd.MarketPrice*float64(teamState.PointsEarned) -
		rd.CostOfPublishing*float64(teamState.PublishedJokes) -
		rd.CostOfDiscard*float64(teamState.DiscardedJokes)

	type profitRow struct {
		teamID int64
		profit float64
	}
	var rows []profitRow
	for key, s := range st.TeamState {
		if key[0] != roundID {
			continue
		}
		p := rd.MarketPrice*float64(s.PointsEarned) -
			rd.CostOfPublishing*float64(s.PublishedJokes) -
			rd.CostOfDiscard*float64(s.DiscardedJokes)
		rows = append(rows, profitRow{teamID: key[1], profit: p})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].profit == rows[j].profit {
			return rows[i].teamID < rows[j].teamID
		}
		return rows[i].profit > rows[j].profit
	})
	rank := 1
	for _, r := range rows {
		if r.teamID == teamID {
			break
		}
		rank++
	}

	unprocessed, _ := st.CountSubmittedBatchesForTeam(ctx, roundID, teamID)
	sold := teamState.PointsEarned
	if sold > teamState.PublishedJokes {
		sold = teamState.PublishedJokes
	}
	unsold := teamState.PublishedJokes - sold
	if unsold < 0 {
		unsold = 0
	}
	return &ports.TeamSummary{
		Team: *team, RoundID: roundID, Rank: rank,
		Points: teamState.PointsEarned, Profit: profit, TotalSales: teamState.PointsEarned,
		Performance:    "AVERAGE PERFORMING",
		BatchesCreated: teamState.BatchesCreated, BatchesProcessed: teamState.BatchesProcessed,
		PublishedJokes: teamState.PublishedJokes, DiscardedJokes: teamState.DiscardedJokes,
		UnsoldJokes: unsold, SoldJokesCount: sold, UnprocessedBatches: unprocessed,
	}, nil
}

func (st *Store) GetLobby(ctx context.Context, roundID int64) (*ports.LobbySnapshot, error) {
	snap := &ports.LobbySnapshot{RoundID: roundID}
	for _, t := range st.Teams {
		members, _ := st.ListTeamMembers(ctx, t.ID)
		if len(members) > 0 {
			snap.Teams = append(snap.Teams, ports.LobbyTeam{Team: t, Members: members})
			snap.Summary.Assigned += len(members)
		}
	}
	snap.Summary.TeamCount = len(snap.Teams)
	waiting, _ := st.ListUsersByStatus(ctx, domain.ParticipantWaiting)
	snap.Summary.Waiting = len(waiting)
	for _, u := range waiting {
		snap.Unassigned = append(snap.Unassigned, ports.LobbyUnassigned{
			UserID: u.ID, DisplayName: u.DisplayName, Status: u.Status,
		})
	}
	return snap, nil
}

func (st *Store) GetRoundStatsV2(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	rd, ok := st.Rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	stats := &ports.RoundStats{RoundID: roundID, Leaderboard: []ports.TeamStats{}}
	type row struct {
		ts     ports.TeamStats
		profit float64
	}
	var rows []row
	for key, state := range st.TeamState {
		if key[0] != roundID {
			continue
		}
		team, err := st.GetTeam(ctx, key[1])
		if err != nil {
			continue
		}
		profit := rd.MarketPrice*float64(state.PointsEarned) -
			rd.CostOfPublishing*float64(state.PublishedJokes) -
			rd.CostOfDiscard*float64(state.DiscardedJokes)
		unsold := state.PublishedJokes - state.PointsEarned
		if unsold < 0 {
			unsold = 0
		}
		rows = append(rows, row{
			profit: profit,
			ts: ports.TeamStats{
				Team: *team, BatchesProcessed: state.BatchesProcessed,
				TotalSales: state.PointsEarned, PublishedJokes: state.PublishedJokes,
				DiscardedJokes: state.DiscardedJokes,
				TotalJokes:     state.PublishedJokes + state.DiscardedJokes,
				UnsoldJokes:    unsold, Profit: profit,
			},
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].profit == rows[j].profit {
			return rows[i].ts.Team.ID < rows[j].ts.Team.ID
		}
		return rows[i].profit > rows[j].profit
	})
	for i := range rows {
		rows[i].ts.Rank = i + 1
		stats.Leaderboard = append(stats.Leaderboard, rows[i].ts)
	}
	return stats, nil
}

func (st *Store) ListTeamFeedbackJokes(_ context.Context, roundID, teamID int64, limit int) ([]ports.FeedbackJokeRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	type candidate struct {
		joke      domain.Joke
		published time.Time
	}
	var cands []candidate
	for _, b := range st.Batches {
		if b.RoundID != roundID || b.TeamID != teamID {
			continue
		}
		for _, j := range b.Jokes {
			if j.PublishStatus != domain.PublishPublished || j.PublishedAt == nil {
				continue
			}
			cands = append(cands, candidate{joke: j, published: *j.PublishedAt})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if !cands[i].published.Equal(cands[j].published) {
			return cands[i].published.After(cands[j].published)
		}
		return cands[i].joke.ID > cands[j].joke.ID
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}

	bought := make(map[int64]bool)
	for _, p := range st.Purchases {
		if p.RoundID == roundID {
			bought[p.JokeID] = true
		}
	}

	out := make([]ports.FeedbackJokeRow, 0, len(cands))
	for _, c := range cands {
		title := ""
		if c.joke.Title != nil {
			title = *c.joke.Title
		}
		fits := make(map[domain.Dimension]float64)
		if src, ok := st.DimFits[c.joke.ID]; ok {
			for k, v := range src {
				fits[k] = v
			}
		}
		out = append(out, ports.FeedbackJokeRow{
			JokeID:    c.joke.ID,
			JokeTitle: title,
			WasBought: bought[c.joke.ID],
			DimFits:   fits,
		})
	}
	return out, nil
}

func applyConfig(r *domain.Round, cfg *domain.RoundConfig) {
	r.CustomerBudget = cfg.CustomerBudget
	r.BatchSize = cfg.BatchSize
	r.MarketPrice = cfg.MarketPrice
	r.CostOfPublishing = cfg.CostOfPublishing
	r.CostOfDiscard = cfg.CostOfDiscard
	r.CustomerCount = cfg.CustomerCount
	r.BuyThreshold = cfg.BuyThreshold
	r.Jitter = cfg.Jitter
	r.SwapMargin = cfg.SwapMargin
	r.FeedbackJokeCount = cfg.FeedbackJokeCount
	r.FeedbackPassThreshold = cfg.FeedbackPassThreshold
}

func cloneUser(u *domain.User) *domain.User {
	cp := *u
	if u.Role != nil {
		r := *u.Role
		cp.Role = &r
	}
	if u.TeamID != nil {
		t := *u.TeamID
		cp.TeamID = &t
	}
	if u.AssignedAt != nil {
		a := *u.AssignedAt
		cp.AssignedAt = &a
	}
	return &cp
}

func cloneRound(r *domain.Round) *domain.Round {
	cp := *r
	if r.StartedAt != nil {
		t := *r.StartedAt
		cp.StartedAt = &t
	}
	if r.EndedAt != nil {
		t := *r.EndedAt
		cp.EndedAt = &t
	}
	return &cp
}

func cloneBatch(b *domain.Batch) *domain.Batch {
	cp := *b
	cp.Jokes = append([]domain.Joke(nil), b.Jokes...)
	if b.SubmittedAt != nil {
		t := *b.SubmittedAt
		cp.SubmittedAt = &t
	}
	if b.ProcessedAt != nil {
		t := *b.ProcessedAt
		cp.ProcessedAt = &t
	}
	if b.LockedAt != nil {
		t := *b.LockedAt
		cp.LockedAt = &t
	}
	if b.LockedBy != nil {
		t := *b.LockedBy
		cp.LockedBy = &t
	}
	return &cp
}

func itoa(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}
