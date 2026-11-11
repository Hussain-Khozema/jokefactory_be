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

// Listing (cost_of_publishing) and discard (cost_of_discard) are fees like
// hosting: free to change before a Lean round starts, locked after it (a 409
// MARKET_MODE_LOCKED), and re-sending the stored value stays allowed. Other
// fields on the same round still save after the start.
func TestListingAndDiscardLockOnceStarted(t *testing.T) {
	st := testutil.NewStore()
	cfg := &config.Config{Admin: config.AdminConfig{AdminPassword: "pw"}}
	h := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st, nil, nil).Router()

	code, login := feesRequest(t, h, http.MethodPost, "/v1/instructor/login", 0,
		map[string]any{"display_name": "Instructor", "password": "pw"})
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, login)
	}
	instructor := int64(login["user"].(map[string]any)["user_id"].(float64))
	post := func(path string, body map[string]any) (int, map[string]any) {
		t.Helper()
		return feesRequest(t, h, http.MethodPost, path, instructor, body)
	}

	if code, body := post("/v1/instructor/rounds/1/config", map[string]any{"cost_of_publishing": 0.25, "cost_of_discard": 0.05}); code != http.StatusOK {
		t.Fatalf("before start: %d %v; want 200", code, body)
	}
	if code, body := post("/v1/instructor/rounds/1/start", map[string]any{
		"ideal_profile": dto.IdealProfileFromDomain(testutil.ValidIdealProfile()),
	}); code != http.StatusOK {
		t.Fatalf("start: %d %v", code, body)
	}

	for _, change := range []map[string]any{{"cost_of_publishing": 0.5}, {"cost_of_discard": 0}} {
		code, body := post("/v1/instructor/rounds/1/config", change)
		e, _ := body["error"].(map[string]any)
		msg, _ := e["message"].(string)
		if code != http.StatusConflict || !strings.Contains(msg, "MARKET_MODE_LOCKED") {
			t.Errorf("change %v after start: %d %v; want 409 MARKET_MODE_LOCKED", change, code, body)
		}
	}
	code, body := post("/v1/instructor/rounds/1/config", map[string]any{"cost_of_publishing": 0.25, "cost_of_discard": 0.05, "market_price": 2})
	if code != http.StatusOK {
		t.Fatalf("re-send after start: %d %v; want 200", code, body)
	}
	r := body["data"].(map[string]any)["round"].(map[string]any)
	if r["cost_of_publishing"] != 0.25 || r["cost_of_discard"] != 0.05 || r["market_price"] != 2.0 {
		t.Errorf("round 1 after re-send = %v; want listing 0.25, discard 0.05, price 2", r)
	}
}
