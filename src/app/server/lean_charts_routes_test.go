package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"jokefactory/src/app/server"
	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/config"
)

// leanChartsStore seeds two started rounds on two teams: round 1 LEAN with one
// batch from team 1, round 2 CLASSIC. It returns the store and an instructor id.
func leanChartsStore(t *testing.T) (*testutil.Store, int64) {
	t.Helper()
	ctx := context.Background()
	st := testutil.NewStore()
	st.Teams = []domain.Team{{ID: 1, Name: "Team 1"}, {ID: 2, Name: "Team 2"}}
	lean := domain.DefaultRoundConfig()
	lean.MarketMode = domain.MarketLean
	lean.SetupCostPerBatch = 2
	lean.CarryingCostPerJokeMinute = 0.05
	if _, err := st.InsertRoundConfig(ctx, 1, &lean); err != nil {
		t.Fatal(err)
	}
	classic := domain.DefaultRoundConfig()
	if _, err := st.InsertRoundConfig(ctx, 2, &classic); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		if _, err := st.StartRound(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateBatch(ctx, 1, 1, []string{"a joke", "another joke"}, ""); err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "Instructor")
	if err != nil {
		t.Fatal(err)
	}
	role := domain.RoleInstructor
	if err := st.UpdateUserAssignment(ctx, u.ID, &role, nil); err != nil {
		t.Fatal(err)
	}
	return st, u.ID
}

func leanChartsGet(t *testing.T, h http.Handler, path string, userID int64) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if userID != 0 {
		req.Header.Set("X-User-Id", strconv.FormatInt(userID, 10))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, rec.Body.String())
	}
	return rec.Code, body
}

func TestLeanChartsRoutes(t *testing.T) {
	st, instructor := leanChartsStore(t)
	h := server.New(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), st, nil, nil).Router()

	if code, _ := leanChartsGet(t, h, "/v1/instructor/rounds/1/lean-charts", 0); code != http.StatusUnauthorized {
		t.Fatalf("instructor route without X-User-Id: %d; want 401", code)
	}
	code, body := leanChartsGet(t, h, "/v1/instructor/rounds/1/lean-charts", instructor)
	if code != http.StatusOK {
		t.Fatalf("instructor route: %d %v", code, body)
	}
	data := body["data"].(map[string]any)
	if teams := data["teams"].([]any); len(teams) != 2 || data["round_id"].(float64) != 1 {
		t.Fatalf("instructor route data %v; want round 1 with both teams", data)
	}

	code, body = leanChartsGet(t, h, "/v1/rounds/1/teams/2/lean-charts", 0)
	if code != http.StatusOK {
		t.Fatalf("team route: %d %v", code, body)
	}
	teams := body["data"].(map[string]any)["teams"].([]any)
	if len(teams) != 1 || teams[0].(map[string]any)["team_id"].(float64) != 2 {
		t.Fatalf("team route teams %v; want exactly team 2", teams)
	}

	for _, path := range []string{"/v1/instructor/rounds/2/lean-charts", "/v1/rounds/2/teams/1/lean-charts"} {
		code, body = leanChartsGet(t, h, path, instructor)
		e := body["error"].(map[string]any)
		if code != http.StatusConflict || e["code"] != "CONFLICT" || !strings.Contains(e["message"].(string), "NOT_LEAN_ROUND") {
			t.Fatalf("%s on a Classic round: %d %v; want 409 CONFLICT NOT_LEAN_ROUND", path, code, body)
		}
	}
	for _, path := range []string{"/v1/instructor/rounds/99/lean-charts", "/v1/rounds/99/teams/1/lean-charts", "/v1/rounds/1/teams/99/lean-charts"} {
		if code, body = leanChartsGet(t, h, path, instructor); code != http.StatusNotFound {
			t.Fatalf("%s: %d %v; want 404", path, code, body)
		}
	}
	for _, path := range []string{"/v1/instructor/rounds/x/lean-charts", "/v1/rounds/1/teams/x/lean-charts"} {
		if code, body = leanChartsGet(t, h, path, instructor); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %v; want 400", path, code, body)
		}
	}
}
