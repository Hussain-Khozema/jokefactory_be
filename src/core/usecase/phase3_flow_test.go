package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
)

func TestPhase3PreMarketingFlow(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)

	// Seed a configured round shell (as instructor login would).
	defaults := domain.DefaultRoundConfig()
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatalf("seed round: %v", err)
	}

	// Join students.
	names := []string{"Alice", "Bob", "Carol", "Dave"}
	users := make([]*domain.User, 0, len(names))
	for _, name := range names {
		res, err := session.Join(ctx, name)
		if err != nil {
			t.Fatalf("join %s: %v", name, err)
		}
		if res.User.Status != domain.ParticipantWaiting {
			t.Fatalf("expected WAITING, got %s", res.User.Status)
		}
		users = append(users, res.User)
	}

	// Assign: 2 teams → JM + MARKETING each.
	lobby, err := instructor.Assign(ctx, 1, 2)
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if lobby.Summary.TeamCount != 2 {
		t.Fatalf("expected 2 teams, got %d", lobby.Summary.TeamCount)
	}
	if lobby.Summary.Assigned != 4 {
		t.Fatalf("expected 4 assigned, got %d", lobby.Summary.Assigned)
	}

	jm := findJM(t, session, users)

	// Configure round + ideal profile.
	cfg := domain.DefaultRoundConfig()
	cfg.BatchSize = 2
	cfg.BuyThreshold = 7.5
	profile := validIdealProfile()
	result, err := instructor.Config(ctx, 1, &cfg, profile)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if result.Round.BatchSize != 2 || result.Round.BuyThreshold != 7.5 {
		t.Fatalf("config not persisted: %+v", result.Round)
	}
	if len(result.IdealProfile) != len(scoring.IdealDimensions()) {
		t.Fatalf("ideal profile size = %d", len(result.IdealProfile))
	}

	// Start.
	started, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Status != domain.RoundActive {
		t.Fatalf("expected ACTIVE, got %s", started.Status)
	}

	// JM submits batch.
	jokes := []string{"Why did the chicken cross the road?", "Because it could."}
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, jokes)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if batch.Status != domain.BatchSubmitted {
		t.Fatalf("expected SUBMITTED, got %s", batch.Status)
	}

	listed, err := batches.List(ctx, 1, *jm.TeamID, jm.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || len(listed[0].Jokes) != 2 {
		t.Fatalf("list unexpected: %+v", listed)
	}
}

func findJM(t *testing.T, session *usecase.SessionService, users []*domain.User) *domain.User {
	t.Helper()
	ctx := context.Background()
	for _, u := range users {
		me, err := session.Me(ctx, u.ID)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		if me.User.Role != nil && *me.User.Role == domain.RoleJM {
			return me.User
		}
	}
	t.Fatal("expected a JM with team")
	return nil
}

func TestStartRequiresIdealProfile(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	instructor := usecase.NewInstructorService(store, nil, log)

	defaults := domain.DefaultRoundConfig()
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatal(err)
	}
	_, err := instructor.StartRound(ctx, 1)
	if err == nil || !domain.IsConflict(err) {
		t.Fatalf("expected conflict without ideal profile, got %v", err)
	}
}

func TestConfigRejectsInvalidIdeal(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	instructor := usecase.NewInstructorService(store, nil, log)

	bad := validIdealProfile()
	bad[domain.DimTopic] = "NotARealTopic"
	cfg := domain.DefaultRoundConfig()
	_, err := instructor.Config(ctx, 1, &cfg, bad)
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func validIdealProfile() domain.IdealProfile {
	return domain.IdealProfile{
		domain.DimLength:      scoring.LengthMedium,
		domain.DimTopic:       "Work",
		domain.DimHumorStyle:  "Observational",
		domain.DimComplexity:  "Moderate",
		domain.DimEdginess:    "Clean",
		domain.DimStructure:   "Setup–punchline",
		domain.DimWordplay:    "Light",
		domain.DimFreshness:   "Timeless",
		domain.DimSetupPayoff: "Balanced",
		domain.DimClarity:     "Crystal clear",
		domain.DimEnergy:      "Conversational",
	}
}

// --- in-memory Store for Phase 3 flow tests ---

type memStore struct {
	users        map[int64]*domain.User
	byName       map[string]int64
	teams        []domain.Team
	rounds       map[int64]*domain.Round
	ideals       map[int64]domain.IdealProfile
	teamState    map[[2]int64]*domain.TeamRoundState
	batches      map[int64]*domain.Batch
	classJobs    map[int64]*domain.ClassificationJob
	dimValues    map[int64]map[domain.Dimension]string
	dimFits      map[int64]map[domain.Dimension]float64
	jokeFits     map[int64]*domain.JokeFit
	aiCustomers  map[int64]*domain.AICustomer // keyed by ai_customer_id
	purchases    map[int64]*domain.Purchase   // keyed by purchase_id
	nextUser     int64
	nextTeam     int64
	nextBatch    int64
	nextJoke     int64
	nextAICust   int64
	nextPurchase int64
}

func newMemStore() *memStore {
	return &memStore{
		users:        make(map[int64]*domain.User),
		byName:       make(map[string]int64),
		rounds:       make(map[int64]*domain.Round),
		ideals:       make(map[int64]domain.IdealProfile),
		teamState:    make(map[[2]int64]*domain.TeamRoundState),
		batches:      make(map[int64]*domain.Batch),
		classJobs:    make(map[int64]*domain.ClassificationJob),
		dimValues:    make(map[int64]map[domain.Dimension]string),
		dimFits:      make(map[int64]map[domain.Dimension]float64),
		jokeFits:     make(map[int64]*domain.JokeFit),
		aiCustomers:  make(map[int64]*domain.AICustomer),
		purchases:    make(map[int64]*domain.Purchase),
		nextUser:     1,
		nextTeam:     1,
		nextBatch:    1,
		nextJoke:     1,
		nextAICust:   1,
		nextPurchase: 1,
	}
}

func (m *memStore) Health(context.Context) error { return nil }

func (m *memStore) CreateUser(_ context.Context, displayName string) (*domain.User, error) {
	id := m.nextUser
	m.nextUser++
	now := time.Now().UTC()
	u := &domain.User{
		ID: id, DisplayName: displayName, Status: domain.ParticipantWaiting,
		JoinedAt: now, CreatedAt: now,
	}
	m.users[id] = u
	m.byName[displayName] = id
	return cloneUser(u), nil
}

func (m *memStore) GetUserByDisplayName(_ context.Context, displayName string) (*domain.User, error) {
	id, ok := m.byName[displayName]
	if !ok {
		return nil, domain.NewNotFoundError("user")
	}
	return cloneUser(m.users[id]), nil
}

func (m *memStore) GetUserByID(_ context.Context, userID int64) (*domain.User, error) {
	u, ok := m.users[userID]
	if !ok {
		return nil, domain.NewNotFoundError("user")
	}
	return cloneUser(u), nil
}

func (m *memStore) UpdateUserAssignment(_ context.Context, userID int64, role *domain.Role, teamID *int64) error {
	u, ok := m.users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Role = role
	u.TeamID = teamID
	return nil
}

func (m *memStore) UpdateUserStatus(_ context.Context, userID int64, status domain.ParticipantStatus) error {
	u, ok := m.users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Status = status
	return nil
}

func (m *memStore) PatchUserInRound(_ context.Context, _, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) error {
	u, ok := m.users[userID]
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

func (m *memStore) MarkUserAssigned(_ context.Context, userID int64) error {
	u, ok := m.users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	u.Status = domain.ParticipantAssigned
	now := time.Now().UTC()
	u.AssignedAt = &now
	return nil
}

func (m *memStore) ListUsersByStatus(_ context.Context, status domain.ParticipantStatus) ([]domain.User, error) {
	var out []domain.User
	for _, u := range m.users {
		if u.Status == status && (u.Role == nil || *u.Role != domain.RoleInstructor) {
			out = append(out, *cloneUser(u))
		}
	}
	return out, nil
}

func (m *memStore) ListTeamMembers(_ context.Context, teamID int64) ([]ports.TeamMember, error) {
	var out []ports.TeamMember
	for _, u := range m.users {
		if u.TeamID != nil && *u.TeamID == teamID && u.Role != nil {
			out = append(out, ports.TeamMember{UserID: u.ID, DisplayName: u.DisplayName, Role: *u.Role})
		}
	}
	return out, nil
}

func (m *memStore) DeleteUser(_ context.Context, userID int64) error {
	u, ok := m.users[userID]
	if !ok {
		return domain.NewNotFoundError("user")
	}
	if u.Role != nil && *u.Role == domain.RoleInstructor {
		return domain.NewConflictError("cannot delete instructor user")
	}
	delete(m.byName, u.DisplayName)
	delete(m.users, userID)
	return nil
}

func (m *memStore) EnsureTeamCount(_ context.Context, teamCount int) ([]domain.Team, error) {
	for len(m.teams) < teamCount {
		id := m.nextTeam
		m.nextTeam++
		m.teams = append(m.teams, domain.Team{ID: id, Name: "Team " + itoa(id), CreatedAt: time.Now().UTC()})
	}
	out := make([]domain.Team, len(m.teams))
	copy(out, m.teams)
	return out, nil
}

func (m *memStore) GetTeam(_ context.Context, teamID int64) (*domain.Team, error) {
	for i := range m.teams {
		if m.teams[i].ID == teamID {
			t := m.teams[i]
			return &t, nil
		}
	}
	return nil, domain.NewNotFoundError("team")
}

func (m *memStore) GetActiveRound(_ context.Context) (*domain.Round, error) {
	for _, r := range m.rounds {
		if r.Status == domain.RoundActive {
			return cloneRound(r), nil
		}
	}
	// Intentionally nil,nil — mirrors postgres.GetActiveRound when none active.
	return nil, nil //nolint:nilnil
}

func (m *memStore) GetRoundByID(_ context.Context, roundID int64) (*domain.Round, error) {
	r, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	return cloneRound(r), nil
}

func (m *memStore) GetLatestRound(_ context.Context) (*domain.Round, error) {
	var best *domain.Round
	for _, r := range m.rounds {
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

func (m *memStore) ListRounds(_ context.Context) ([]domain.Round, error) {
	out := make([]domain.Round, 0, len(m.rounds))
	for _, r := range m.rounds {
		out = append(out, *cloneRound(r))
	}
	return out, nil
}

func (m *memStore) UpdateRoundConfig(_ context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error) {
	r, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	applyConfig(r, cfg)
	return cloneRound(r), nil
}

func (m *memStore) InsertRoundConfig(_ context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error) {
	if r, ok := m.rounds[roundID]; ok {
		applyConfig(r, cfg)
		return cloneRound(r), nil
	}
	r := &domain.Round{
		ID: roundID, RoundNumber: int(roundID), Status: domain.RoundConfigured, CreatedAt: time.Now().UTC(),
	}
	applyConfig(r, cfg)
	m.rounds[roundID] = r
	return cloneRound(r), nil
}

func (m *memStore) UpsertIdealProfile(_ context.Context, roundID int64, profile domain.IdealProfile) error {
	cp := make(domain.IdealProfile, len(profile))
	for k, v := range profile {
		cp[k] = v
	}
	m.ideals[roundID] = cp
	return nil
}

func (m *memStore) GetIdealProfile(_ context.Context, roundID int64) (domain.IdealProfile, error) {
	p := m.ideals[roundID]
	if p == nil {
		return domain.IdealProfile{}, nil
	}
	cp := make(domain.IdealProfile, len(p))
	for k, v := range p {
		cp[k] = v
	}
	return cp, nil
}

func (m *memStore) StartRound(_ context.Context, roundID int64) (*domain.Round, error) {
	r, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	r.Status = domain.RoundActive
	now := time.Now().UTC()
	r.StartedAt = &now
	r.EndedAt = nil
	for _, t := range m.teams {
		m.ensureState(roundID, t.ID)
	}
	return cloneRound(r), nil
}

func (m *memStore) EndRound(_ context.Context, roundID int64) (*domain.Round, error) {
	r, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	r.Status = domain.RoundEnded
	now := time.Now().UTC()
	r.EndedAt = &now
	return cloneRound(r), nil
}

func (m *memStore) SetRoundPopupState(_ context.Context, roundID int64, isActive bool) (*domain.Round, error) {
	r, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	r.IsPoppedActive = isActive
	return cloneRound(r), nil
}

func (m *memStore) EnsureTeamRoundState(_ context.Context, roundID, teamID int64) error {
	m.ensureState(roundID, teamID)
	return nil
}

func (m *memStore) ensureState(roundID, teamID int64) *domain.TeamRoundState {
	key := [2]int64{roundID, teamID}
	if s, ok := m.teamState[key]; ok {
		return s
	}
	now := time.Now().UTC()
	s := &domain.TeamRoundState{RoundID: roundID, TeamID: teamID, CreatedAt: now, UpdatedAt: now}
	m.teamState[key] = s
	return s
}

func (m *memStore) ResetGame(context.Context) error {
	*m = *newMemStore()
	return nil
}

func (m *memStore) CreateBatch(_ context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error) {
	id := m.nextBatch
	m.nextBatch++
	now := time.Now().UTC()
	b := &domain.Batch{
		ID: id, RoundID: roundID, TeamID: teamID, Status: domain.BatchSubmitted,
		SubmittedAt: &now, CreatedAt: now,
	}
	for _, text := range jokes {
		jid := m.nextJoke
		m.nextJoke++
		b.Jokes = append(b.Jokes, domain.Joke{
			ID: jid, BatchID: id, Text: text, PublishStatus: domain.PublishPending, CreatedAt: now,
		})
	}
	m.batches[id] = b
	st := m.ensureState(roundID, teamID)
	st.BatchesCreated++
	st.UpdatedAt = now
	return cloneBatch(b), nil
}

func (m *memStore) ListBatchesByTeam(_ context.Context, roundID, teamID int64) ([]domain.Batch, error) {
	var out []domain.Batch
	for _, b := range m.batches {
		if b.RoundID == roundID && b.TeamID == teamID {
			out = append(out, *cloneBatch(b))
		}
	}
	return out, nil
}

func (m *memStore) GetBatchWithJokes(_ context.Context, batchID int64) (*ports.BatchWithJokes, error) {
	b, ok := m.batches[batchID]
	if !ok {
		return nil, domain.NewNotFoundError("batch")
	}
	cp := cloneBatch(b)
	return &ports.BatchWithJokes{Batch: *cp, Jokes: cp.Jokes}, nil
}

func (m *memStore) CountSubmittedBatches(_ context.Context, roundID int64) (int, error) {
	n := 0
	for _, b := range m.batches {
		if b.RoundID == roundID && b.Status == domain.BatchSubmitted {
			n++
		}
	}
	return n, nil
}

func (m *memStore) CountSubmittedBatchesForTeam(_ context.Context, roundID, teamID int64) (int, error) {
	n := 0
	for _, b := range m.batches {
		if b.RoundID == roundID && b.TeamID == teamID && b.Status == domain.BatchSubmitted {
			n++
		}
	}
	return n, nil
}

func (m *memStore) ClaimNextBatch(_ context.Context, roundID, teamID, marketerID int64) (*ports.BatchWithJokes, error) {
	var held *domain.Batch
	var next *domain.Batch
	for _, b := range m.batches {
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

func (m *memStore) PublishBatch(
	_ context.Context,
	batchID, marketerID, teamID int64,
	decisions []ports.JokePublishDecision,
) (*ports.PublishResult, error) {
	b, ok := m.batches[batchID]
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

	st := m.ensureState(b.RoundID, b.TeamID)
	st.BatchesProcessed++
	st.PublishedJokes += len(published)
	st.DiscardedJokes += len(discarded)
	st.UpdatedAt = now

	if _, ok := m.classJobs[batchID]; !ok {
		m.classJobs[batchID] = &domain.ClassificationJob{
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

func (m *memStore) EnsureClassificationJob(_ context.Context, batchID, roundID int64) error {
	if _, ok := m.classJobs[batchID]; ok {
		return nil
	}
	now := time.Now().UTC()
	m.classJobs[batchID] = &domain.ClassificationJob{
		BatchID: batchID, RoundID: roundID, Status: domain.ClassificationPending,
		CreatedAt: now, UpdatedAt: now,
	}
	return nil
}

func (m *memStore) ClaimClassificationJob(_ context.Context, batchID int64) (*domain.ClassificationJob, error) {
	job, ok := m.classJobs[batchID]
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

func (m *memStore) MarkClassificationDone(_ context.Context, batchID int64, model string) error {
	job, ok := m.classJobs[batchID]
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

func (m *memStore) MarkClassificationFailed(_ context.Context, batchID int64, errMsg string) error {
	job, ok := m.classJobs[batchID]
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

func (m *memStore) PersistJokeFits(_ context.Context, fits []ports.JokeFitMaterialization) error {
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
		m.dimValues[fit.JokeID] = cats
		m.dimFits[fit.JokeID] = scores
		m.jokeFits[fit.JokeID] = &domain.JokeFit{
			JokeID: fit.JokeID, RoundID: fit.RoundID, TrueFit: fit.TrueFit, ComputedAt: now,
		}
	}
	return nil
}

func (m *memStore) GetJokeFit(_ context.Context, jokeID int64) (*domain.JokeFit, error) {
	f, ok := m.jokeFits[jokeID]
	if !ok {
		return nil, domain.NewNotFoundError("joke_fit")
	}
	cp := *f
	return &cp, nil
}

func (m *memStore) ListJokeDimFits(_ context.Context, jokeID int64) ([]domain.JokeDimFit, error) {
	scores, ok := m.dimFits[jokeID]
	if !ok {
		return nil, nil
	}
	out := make([]domain.JokeDimFit, 0, len(scores))
	for dim, score := range scores {
		out = append(out, domain.JokeDimFit{JokeID: jokeID, Dimension: dim, DimFit: score})
	}
	return out, nil
}

func (m *memStore) ListJokeDimensionValues(_ context.Context, jokeID int64) ([]domain.JokeDimensionValue, error) {
	cats, ok := m.dimValues[jokeID]
	if !ok {
		return nil, nil
	}
	out := make([]domain.JokeDimensionValue, 0, len(cats))
	for dim, cat := range cats {
		out = append(out, domain.JokeDimensionValue{JokeID: jokeID, Dimension: dim, Category: cat})
	}
	return out, nil
}

func (m *memStore) ListOrphanClassificationBatchIDs(_ context.Context, limit int) ([]int64, error) {
	var ids []int64
	for _, b := range m.batches {
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
		job, ok := m.classJobs[b.ID]
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

func (m *memStore) ReplaceAICustomers(_ context.Context, roundID int64, customers []domain.AICustomer) error {
	for id, c := range m.aiCustomers {
		if c.RoundID == roundID {
			delete(m.aiCustomers, id)
		}
	}
	now := time.Now().UTC()
	for _, c := range customers {
		id := m.nextAICust
		m.nextAICust++
		cp := c
		cp.ID = id
		cp.RoundID = roundID
		cp.CreatedAt = now
		m.aiCustomers[id] = &cp
	}
	return nil
}

func (m *memStore) ListAICustomers(_ context.Context, roundID int64) ([]domain.AICustomer, error) {
	var out []domain.AICustomer
	for _, c := range m.aiCustomers {
		if c.RoundID == roundID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *memStore) ListCandidateJokes(_ context.Context, jokeIDs []int64) ([]ports.CandidateJoke, error) {
	want := make(map[int64]struct{}, len(jokeIDs))
	for _, id := range jokeIDs {
		want[id] = struct{}{}
	}
	var out []ports.CandidateJoke
	for _, b := range m.batches {
		for _, j := range b.Jokes {
			if _, ok := want[j.ID]; !ok {
				continue
			}
			if j.PublishStatus != domain.PublishPublished {
				continue
			}
			fit, ok := m.jokeFits[j.ID]
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

func (m *memStore) ListHoldings(_ context.Context, roundID, aiCustomerID int64) ([]ports.HeldJoke, error) {
	var out []ports.HeldJoke
	for _, p := range m.purchases {
		if p.RoundID != roundID || p.AICustomerID != aiCustomerID {
			continue
		}
		tf := 0.0
		if f, ok := m.jokeFits[p.JokeID]; ok {
			tf = f.TrueFit
		}
		out = append(out, ports.HeldJoke{
			JokeID: p.JokeID, TeamID: p.TeamID, TrueFit: tf, Price: p.Price,
		})
	}
	return out, nil
}

func (m *memStore) BuyJoke(_ context.Context, roundID, aiCustomerID, jokeID, teamID int64, price float64) error {
	c, ok := m.aiCustomers[aiCustomerID]
	if !ok || c.RoundID != roundID {
		return domain.NewNotFoundError("ai_customer")
	}
	for _, p := range m.purchases {
		if p.RoundID == roundID && p.AICustomerID == aiCustomerID && p.JokeID == jokeID {
			return nil
		}
	}
	if c.RemainingBudget < price {
		return domain.NewConflictError("insufficient budget")
	}
	c.RemainingBudget -= price
	id := m.nextPurchase
	m.nextPurchase++
	now := time.Now().UTC()
	m.purchases[id] = &domain.Purchase{
		ID: id, RoundID: roundID, AICustomerID: aiCustomerID,
		JokeID: jokeID, TeamID: teamID, Price: price, CreatedAt: now,
	}
	st := m.ensureState(roundID, teamID)
	st.PointsEarned++
	st.UpdatedAt = now
	return nil
}

func (m *memStore) SwapJoke(
	_ context.Context,
	roundID, aiCustomerID, buyJokeID, buyTeamID, returnJokeID, returnTeamID int64,
	price float64,
) error {
	if _, ok := m.aiCustomers[aiCustomerID]; !ok {
		return domain.NewNotFoundError("ai_customer")
	}
	var returned bool
	for id, p := range m.purchases {
		if p.RoundID == roundID && p.AICustomerID == aiCustomerID && p.JokeID == returnJokeID {
			delete(m.purchases, id)
			returned = true
			break
		}
	}
	if !returned {
		return domain.NewConflictError("held joke not found for swap")
	}
	stRet := m.ensureState(roundID, returnTeamID)
	if stRet.PointsEarned > 0 {
		stRet.PointsEarned--
	}
	for _, p := range m.purchases {
		if p.RoundID == roundID && p.AICustomerID == aiCustomerID && p.JokeID == buyJokeID {
			return domain.NewConflictError("already holds buy target")
		}
	}
	id := m.nextPurchase
	m.nextPurchase++
	now := time.Now().UTC()
	m.purchases[id] = &domain.Purchase{
		ID: id, RoundID: roundID, AICustomerID: aiCustomerID,
		JokeID: buyJokeID, TeamID: buyTeamID, Price: price, CreatedAt: now,
	}
	stBuy := m.ensureState(roundID, buyTeamID)
	stBuy.PointsEarned++
	stBuy.UpdatedAt = now
	stRet.UpdatedAt = now
	return nil
}

func (m *memStore) ListMarket(_ context.Context, roundID int64) ([]ports.MarketJoke, error) {
	sold := make(map[int64]int)
	for _, p := range m.purchases {
		if p.RoundID == roundID {
			sold[p.JokeID]++
		}
	}
	teamName := func(id int64) string {
		for _, t := range m.teams {
			if t.ID == id {
				return t.Name
			}
		}
		return ""
	}
	var out []ports.MarketJoke
	for _, b := range m.batches {
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

func (m *memStore) GetTeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	rd, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	team, err := m.GetTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	st := m.ensureState(roundID, teamID)
	profit := rd.MarketPrice*float64(st.PointsEarned) -
		rd.CostOfPublishing*float64(st.PublishedJokes) -
		rd.CostOfDiscard*float64(st.DiscardedJokes)

	type profitRow struct {
		teamID int64
		profit float64
	}
	var rows []profitRow
	for key, s := range m.teamState {
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

	unprocessed, _ := m.CountSubmittedBatchesForTeam(ctx, roundID, teamID)
	sold := st.PointsEarned
	if sold > st.PublishedJokes {
		sold = st.PublishedJokes
	}
	unsold := st.PublishedJokes - sold
	if unsold < 0 {
		unsold = 0
	}
	return &ports.TeamSummary{
		Team: *team, RoundID: roundID, Rank: rank,
		Points: st.PointsEarned, Profit: profit, TotalSales: st.PointsEarned,
		Performance:    "AVERAGE PERFORMING",
		BatchesCreated: st.BatchesCreated, BatchesProcessed: st.BatchesProcessed,
		PublishedJokes: st.PublishedJokes, DiscardedJokes: st.DiscardedJokes,
		UnsoldJokes: unsold, SoldJokesCount: sold, UnprocessedBatches: unprocessed,
	}, nil
}

func (m *memStore) GetLobby(ctx context.Context, roundID int64) (*ports.LobbySnapshot, error) {
	snap := &ports.LobbySnapshot{RoundID: roundID}
	for _, t := range m.teams {
		members, _ := m.ListTeamMembers(ctx, t.ID)
		if len(members) > 0 {
			snap.Teams = append(snap.Teams, ports.LobbyTeam{Team: t, Members: members})
			snap.Summary.Assigned += len(members)
		}
	}
	snap.Summary.TeamCount = len(snap.Teams)
	waiting, _ := m.ListUsersByStatus(ctx, domain.ParticipantWaiting)
	snap.Summary.Waiting = len(waiting)
	for _, u := range waiting {
		snap.Unassigned = append(snap.Unassigned, ports.LobbyUnassigned{
			UserID: u.ID, DisplayName: u.DisplayName, Status: u.Status,
		})
	}
	return snap, nil
}

func (m *memStore) GetRoundStatsV2(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	rd, ok := m.rounds[roundID]
	if !ok {
		return nil, domain.NewNotFoundError("round")
	}
	stats := &ports.RoundStats{RoundID: roundID, Leaderboard: []ports.TeamStats{}}
	type row struct {
		ts     ports.TeamStats
		profit float64
	}
	var rows []row
	for key, st := range m.teamState {
		if key[0] != roundID {
			continue
		}
		team, err := m.GetTeam(ctx, key[1])
		if err != nil {
			continue
		}
		profit := rd.MarketPrice*float64(st.PointsEarned) -
			rd.CostOfPublishing*float64(st.PublishedJokes) -
			rd.CostOfDiscard*float64(st.DiscardedJokes)
		unsold := st.PublishedJokes - st.PointsEarned
		if unsold < 0 {
			unsold = 0
		}
		rows = append(rows, row{
			profit: profit,
			ts: ports.TeamStats{
				Team: *team, BatchesProcessed: st.BatchesProcessed,
				TotalSales: st.PointsEarned, PublishedJokes: st.PublishedJokes,
				DiscardedJokes: st.DiscardedJokes,
				TotalJokes:     st.PublishedJokes + st.DiscardedJokes,
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

func (m *memStore) ListTeamFeedbackJokes(_ context.Context, roundID, teamID int64, limit int) ([]ports.FeedbackJokeRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	type candidate struct {
		joke      domain.Joke
		published time.Time
	}
	var cands []candidate
	for _, b := range m.batches {
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
	for _, p := range m.purchases {
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
		if src, ok := m.dimFits[c.joke.ID]; ok {
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
