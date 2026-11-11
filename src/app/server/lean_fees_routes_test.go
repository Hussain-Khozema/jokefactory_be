package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"jokefactory/src/app/server"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/config"
)

func feesRequest(t *testing.T, h http.Handler, method, path string, userID int64, body any) (int, map[string]any) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		req.Header.Set("X-User-Id", strconv.FormatInt(userID, 10))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: %v\n%s", method, path, err, rec.Body.String())
	}
	return rec.Code, out
}

// The five fee keys travel on the config request and come back on the
// instructor round and on the students' public round; a bad value is a 400 on
// its field; Reset Game restores the defaults with Round 2 batches up to 10.
func TestLeanFeesRoutes(t *testing.T) {
	st := testutil.NewStore()
	cfg := &config.Config{Admin: config.AdminConfig{AdminPassword: "pw"}}
	h := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st, nil, nil).Router()

	code, login := feesRequest(t, h, http.MethodPost, "/v1/instructor/login", 0,
		map[string]any{"display_name": "Instructor", "password": "pw"})
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, login)
	}
	instructor := int64(login["user"].(map[string]any)["user_id"].(float64))

	fees := map[string]any{
		"shipping_fee_small": 0.5, "shipping_fee_standard": 1.5, "shipping_fee_large": 2.5,
		"storage_fee_per_joke_minute": 0.2, "hosting_fee_per_joke_minute": 0.03,
	}
	code, body := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/2/config", instructor, fees)
	if code != http.StatusOK {
		t.Fatalf("config: %d %v", code, body)
	}
	round := body["data"].(map[string]any)["round"].(map[string]any)
	for k, v := range fees {
		if round[k] != v {
			t.Errorf("instructor round %s = %v; want %v", k, round[k], v)
		}
	}
	if round["batch_size"].(float64) != 10 || round["market_mode"] != "LEAN" {
		t.Errorf("round 2 batch_size %v, market_mode %v; want 10, LEAN", round["batch_size"], round["market_mode"])
	}

	publicRound := func(number float64) map[string]any {
		t.Helper()
		code, body := feesRequest(t, h, http.MethodGet, "/v1/rounds/active", 0, nil)
		if code != http.StatusOK {
			t.Fatalf("rounds/active: %d %v", code, body)
		}
		for _, r := range body["data"].(map[string]any)["rounds"].([]any) {
			if m := r.(map[string]any); m["round_number"] == number {
				return m
			}
		}
		t.Fatalf("round %v not listed", number)
		return nil
	}
	r2 := publicRound(2)
	for k, v := range fees {
		if r2[k] != v {
			t.Errorf("public round 2 %s = %v; want %v", k, r2[k], v)
		}
	}
	if r1 := publicRound(1); r1["shipping_fee_standard"] != 0.0 || r1["storage_fee_per_joke_minute"] != 0.0 ||
		r1["hosting_fee_per_joke_minute"] != 0.05 || r1["batch_size"] != 5.0 {
		t.Errorf("public round 1 = %v; want the default fees and batch size 5", r1)
	}

	for _, tc := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"storage_fee_per_joke_minute": -0.1}, "storage_fee_per_joke_minute"},
		{map[string]any{"shipping_fee_small": 3, "shipping_fee_standard": 2}, "shipping_fee_standard"},
	} {
		code, body := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor, tc.body)
		e, _ := body["error"].(map[string]any)
		if code != http.StatusBadRequest || e["code"] != "VALIDATION_ERROR" || e["field"] != tc.field {
			t.Errorf("config %v: %d %v; want 400 VALIDATION_ERROR on %s", tc.body, code, body, tc.field)
		}
	}

	if code, body := feesRequest(t, h, http.MethodPost, "/v1/admin/reset", instructor, nil); code != http.StatusOK {
		t.Fatalf("reset: %d %v", code, body)
	}
	if r2 := publicRound(2); r2["batch_size"] != 10.0 || r2["shipping_fee_small"] != 0.0 ||
		r2["storage_fee_per_joke_minute"] != 0.0 || r2["status"] != "CONFIGURED" {
		t.Errorf("round 2 after reset = %v; want batch_size 10 and the default fees", r2)
	}
}
