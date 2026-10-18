package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"testing"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/server"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/config"
)

// GET /instructor/rounds/:round_id/config reads back the saved round and its
// ideal profile in the same shape the POST returns; it is read-only and
// instructor-only.
func TestInstructorConfigReadRoutes(t *testing.T) {
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

	// No profile saved yet: the round comes back without an ideal_profile key.
	code, body := feesRequest(t, h, http.MethodGet, "/v1/instructor/rounds/1/config", instructor, nil)
	if code != http.StatusOK {
		t.Fatalf("get config: %d %v", code, body)
	}
	r := roundOf(body)
	if r["id"] != 1.0 {
		t.Errorf("round id = %v; want 1", r["id"])
	}
	if _, ok := r["ideal_profile"]; ok {
		t.Errorf("ideal_profile present before any was saved: %v", r["ideal_profile"])
	}

	// Save a profile, then read it back unchanged.
	code, posted := feesRequest(t, h, http.MethodPost, "/v1/instructor/rounds/1/config", instructor,
		map[string]any{"ideal_profile": dto.IdealProfileFromDomain(testutil.ValidIdealProfile())})
	if code != http.StatusOK {
		t.Fatalf("post config: %d %v", code, posted)
	}
	want := roundOf(posted)["ideal_profile"]
	if want == nil {
		t.Fatalf("POST returned no ideal_profile: %v", posted)
	}
	code, body = feesRequest(t, h, http.MethodGet, "/v1/instructor/rounds/1/config", instructor, nil)
	if code != http.StatusOK {
		t.Fatalf("get config after save: %d %v", code, body)
	}
	if got := roundOf(body)["ideal_profile"]; !reflect.DeepEqual(got, want) {
		t.Errorf("ideal_profile = %v; want %v", got, want)
	}

	code, body = feesRequest(t, h, http.MethodGet, "/v1/instructor/rounds/99/config", instructor, nil)
	if e, _ := body["error"].(map[string]any); code != http.StatusNotFound || e["code"] != "NOT_FOUND" {
		t.Errorf("unknown round: %d %v; want 404 NOT_FOUND", code, body)
	}

	if code, body = feesRequest(t, h, http.MethodGet, "/v1/instructor/rounds/abc/config", instructor, nil); code != http.StatusBadRequest {
		t.Errorf("malformed round id: %d %v; want 400", code, body)
	}

	if code, body = feesRequest(t, h, http.MethodGet, "/v1/instructor/rounds/1/config", 0, nil); code != http.StatusUnauthorized {
		t.Errorf("no X-User-Id: %d %v; want 401", code, body)
	}
}
