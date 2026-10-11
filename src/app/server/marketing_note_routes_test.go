package server_test

import (
	"bytes"
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
	"jokefactory/src/infra/worker"
)

// noteRouteFixture seeds an ACTIVE Round 2 with one team, a Joke Maker, a
// Marketing seat holding the lock on a two-joke batch, and a second batch still
// waiting. It returns the router and the ids the requests need.
func noteRouteFixture(t *testing.T) (http.Handler, int64, int64, int64, []int64) {
	t.Helper()
	ctx := context.Background()
	st := testutil.NewStore()
	st.Teams = []domain.Team{{ID: 1, Name: "Team 1"}}
	cfg := domain.DefaultRoundConfig()
	if _, err := st.InsertRoundConfig(ctx, 2, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartRound(ctx, 2); err != nil {
		t.Fatal(err)
	}
	team := int64(1)
	seat := func(name string, role domain.Role) int64 {
		u, err := st.CreateUser(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateUserAssignment(ctx, u.ID, &role, &team); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	jm := seat("JM", domain.RoleJM)
	mkt := seat("Mkt", domain.RoleMarketing)
	batch, err := st.CreateBatch(ctx, 2, team, []string{"one", "two"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBatch(ctx, 2, team, []string{"three"}, ""); err != nil {
		t.Fatal(err)
	}
	claimed, err := st.ClaimNextBatch(ctx, 2, team, mkt)
	if err != nil || claimed == nil || claimed.Batch.ID != batch.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	ids := []int64{claimed.Jokes[0].ID, claimed.Jokes[1].ID}
	h := server.New(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), st, worker.NoopDispatcher{}, nil).Router()
	return h, jm, mkt, batch.ID, ids
}

func noteRouteDo(t *testing.T, h http.Handler, method, path string, userID int64, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", strconv.FormatInt(userID, 10))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: %v\n%s", method, path, err, rec.Body.String())
	}
	return rec.Code, out
}

func TestMarketingNoteRoutes(t *testing.T) {
	h, jm, mkt, batchID, jokeIDs := noteRouteFixture(t)
	publishPath := "/v1/marketing/batches/" + strconv.FormatInt(batchID, 10) + "/publish"
	jokes := []map[string]any{
		{"joke_id": jokeIDs[0], "joke_title": "Kept", "is_published": true},
		{"joke_id": jokeIDs[1], "joke_title": "", "is_published": false},
	}

	// 501 runes: 400 VALIDATION_ERROR on field note, and the batch is untouched.
	code, body := noteRouteDo(t, h, http.MethodPost, publishPath, mkt, map[string]any{
		"jokes": jokes, "note": strings.Repeat("é", 501),
	})
	e, _ := body["error"].(map[string]any)
	if code != http.StatusBadRequest || e["code"] != "VALIDATION_ERROR" || e["field"] != "note" ||
		e["message"] != "must be at most 500 characters" {
		t.Fatalf("501-rune note: %d %v; want 400 VALIDATION_ERROR note", code, body)
	}

	// A valid note goes through, and HTML in it is stored as plain text.
	const note = "<b>Shorter</b> setups.\nMore puns."
	if code, body = noteRouteDo(t, h, http.MethodPost, publishPath, mkt, map[string]any{
		"jokes": jokes, "note": "  " + note + "  ",
	}); code != http.StatusOK {
		t.Fatalf("publish with note: %d %v", code, body)
	}

	code, body = noteRouteDo(t, h, http.MethodGet, "/v1/rounds/2/teams/1/batches", jm, nil)
	if code != http.StatusOK {
		t.Fatalf("batch list: %d %v", code, body)
	}
	listed := body["data"].(map[string]any)["batches"].([]any)
	seen := 0
	for _, raw := range listed {
		b := raw.(map[string]any)
		got, present := b["marketing_note"]
		if !present {
			t.Fatalf("batch %v has no marketing_note key: %v", b["batch_id"], b)
		}
		if int64(b["batch_id"].(float64)) == batchID {
			if got != note {
				t.Fatalf("published batch marketing_note = %#v; want %q", got, note)
			}
		} else if got != nil {
			t.Fatalf("waiting batch marketing_note = %#v; want null", got)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("listed %d batches; want 2", seen)
	}
}
