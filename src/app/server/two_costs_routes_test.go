package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"jokefactory/src/app/server"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/config"
)

// twoCostsIssue names what differs from the two costs on a round as the API
// returns it: listing $0.10 per released joke and hosting $0.05 per
// joke-minute, every other fee 0. "" when it matches.
func twoCostsIssue(r map[string]any) string {
	want := map[string]float64{
		"cost_of_publishing": 0.1, "hosting_fee_per_joke_minute": 0.05,
		"shipping_fee_small": 0, "shipping_fee_standard": 0, "shipping_fee_large": 0,
		"storage_fee_per_joke_minute": 0, "cost_of_discard": 0,
	}
	for k, v := range want {
		if got, ok := r[k].(float64); !ok || got != v {
			return k
		}
	}
	return ""
}

// Every way a round comes to exist starts with the two costs (spec §2.1):
// admin login (rounds 1 and 2), a config call that creates a round (round 3)
// and Reset Game.
func TestTwoCostsOnEveryNewRound(t *testing.T) {
	st := testutil.NewStore()
	cfg := &config.Config{Admin: config.AdminConfig{AdminPassword: "pw"}}
	h := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st, nil, nil).Router()

	code, login := feesRequest(t, h, http.MethodPost, "/v1/instructor/login", 0,
		map[string]any{"display_name": "Instructor", "password": "pw"})
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, login)
	}
	instructor := int64(login["user"].(map[string]any)["user_id"].(float64))

	publicRounds := func(when string) {
		t.Helper()
		code, body := feesRequest(t, h, http.MethodGet, "/v1/rounds/active", 0, nil)
		if code != http.StatusOK {
			t.Fatalf("%s: rounds/active: %d %v", when, code, body)
		}
		seen := 0
		for _, r := range body["data"].(map[string]any)["rounds"].([]any) {
			m := r.(map[string]any)
			if n := m["round_number"]; n != 1.0 && n != 2.0 {
				continue
			}
			seen++
			if k := twoCostsIssue(m); k != "" {
				t.Errorf("%s: public round %v %s = %v; want the two costs", when, m["round_number"], k, m[k])
			}
		}
		if seen != 2 {
			t.Fatalf("%s: %d of rounds 1 and 2 listed", when, seen)
		}
	}
	publicRounds("after login")

	code, body := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/3/config", instructor, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("config creating round 3: %d %v", code, body)
	}
	r3 := body["data"].(map[string]any)["round"].(map[string]any)
	if k := twoCostsIssue(r3); k != "" {
		t.Errorf("round 3 created by config: %s = %v; want the two costs", k, r3[k])
	}

	code, body = feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor,
		map[string]any{"shipping_fee_small": 1, "shipping_fee_standard": 2, "shipping_fee_large": 3, "storage_fee_per_joke_minute": 0.2})
	if code != http.StatusOK {
		t.Fatalf("config round 1: %d %v", code, body)
	}
	if code, body := feesRequest(t, h, http.MethodPost, "/v1/admin/reset", instructor, nil); code != http.StatusOK {
		t.Fatalf("reset: %d %v", code, body)
	}
	publicRounds("after reset")
}
