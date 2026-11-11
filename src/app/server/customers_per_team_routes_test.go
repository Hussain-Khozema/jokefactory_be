package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/server"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/config"
)

// customers_per_team travels on the config request and the /start body, comes
// back on the instructor round and on every public round (so a reloaded CONFIG
// screen can show it), is a 400 outside 0–1000, and is locked once the round
// has started.
func TestCustomersPerTeamRoutes(t *testing.T) {
	st := testutil.NewStore()
	cfg := &config.Config{Admin: config.AdminConfig{AdminPassword: "pw"}}
	h := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st, nil, nil).Router()

	code, login := feesRequest(t, h, http.MethodPost, "/v1/instructor/login", 0,
		map[string]any{"display_name": "Instructor", "password": "pw"})
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, login)
	}
	instructor := int64(login["user"].(map[string]any)["user_id"].(float64))

	roundOf := func(body map[string]any) map[string]any {
		t.Helper()
		return body["data"].(map[string]any)["round"].(map[string]any)
	}

	code, body := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("empty config: %d %v", code, body)
	}
	if r := roundOf(body); r["customers_per_team"] != 18.0 {
		t.Errorf("round 1 after login: customers_per_team %v; want 18", r["customers_per_team"])
	}

	code, body = feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/2/config", instructor,
		map[string]any{"customers_per_team": 12})
	if code != http.StatusOK {
		t.Fatalf("config: %d %v", code, body)
	}
	if r := roundOf(body); r["customers_per_team"] != 12.0 || r["buy_threshold"] != 8.0 || r["preference_base"] != 5.0 ||
		r["storage_fee_per_joke_minute"] != 0.0 {
		t.Errorf("instructor round 2 = %v; want customers_per_team 12 and the tuned defaults", r)
	}

	code, body = feesRequest(t, h, http.MethodGet, "/v1/rounds/active", 0, nil)
	if code != http.StatusOK {
		t.Fatalf("rounds/active: %d %v", code, body)
	}
	for _, r := range body["data"].(map[string]any)["rounds"].([]any) {
		m := r.(map[string]any)
		want := 18.0 // round 1: the default
		if m["round_number"] == 2.0 {
			want = 12 // round 2: as configured above
		}
		if m["customers_per_team"] != want {
			t.Errorf("public round %v: customers_per_team %v; want %v", m["round_number"], m["customers_per_team"], want)
		}
	}

	for _, v := range []int{-1, 1001} {
		code, body := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor,
			map[string]any{"customers_per_team": v})
		e, _ := body["error"].(map[string]any)
		if code != http.StatusBadRequest || e["code"] != "VALIDATION_ERROR" || e["field"] != "customers_per_team" {
			t.Errorf("customers_per_team %d: %d %v; want 400 VALIDATION_ERROR on customers_per_team", v, code, body)
		}
	}

	// Start round 1 through the /start body; no teams are assigned, so T = 1.
	code, body = feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/start", instructor, map[string]any{
		"customers_per_team": 15,
		"ideal_profile":      dto.IdealProfileFromDomain(testutil.ValidIdealProfile()),
	})
	if code != http.StatusOK {
		t.Fatalf("start: %d %v", code, body)
	}
	if r := roundOf(body); r["customers_per_team"] != 15.0 || r["customer_count"] != 15.0 {
		t.Errorf("started round 1 = %v; want customers_per_team 15, customer_count 15", r)
	}

	code, body = feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor,
		map[string]any{"customers_per_team": 20})
	e, _ := body["error"].(map[string]any)
	if code != http.StatusConflict || e == nil {
		t.Errorf("change after start: %d %v; want 409 MARKET_MODE_LOCKED", code, body)
	} else if msg, _ := e["message"].(string); !strings.Contains(msg, "MARKET_MODE_LOCKED") {
		t.Errorf("change after start: %v; want MARKET_MODE_LOCKED", e)
	}
	if code, body := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor,
		map[string]any{"customers_per_team": 15}); code != http.StatusOK {
		t.Errorf("re-send after start: %d %v; want 200", code, body)
	}
}
