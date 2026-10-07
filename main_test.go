package main

import (
	"strings"
	"testing"

	"jokefactory/src/infra/config"
	"jokefactory/src/infra/llm"
)

func TestBuildClassifierDefaultsWithoutHash(t *testing.T) {
	c, model, err := buildClassifier(config.LLMConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(llm.StubClassifier); !ok || model != "stub" {
		t.Fatalf("no keys, no mode: got %T %q, want stub", c, model)
	}

	c, model, err = buildClassifier(config.LLMConfig{BaseURL: "https://x", APIKey: "k", Deployment: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(llm.HashClassifier); ok || model != "d" {
		t.Fatalf("keys, no mode: got %T %q, want azure deployment", c, model)
	}
}

func TestBuildClassifierHashNeedsConfirmation(t *testing.T) {
	for _, confirm := range []string{"", "yes", "LOCAL-PILOT-ONLY"} {
		_, _, err := buildClassifier(config.LLMConfig{Mode: "hash", HashConfirm: confirm})
		if err == nil || !strings.Contains(err.Error(), "APP_LLM_HASH_CONFIRM") {
			t.Fatalf("confirm %q: want an error naming APP_LLM_HASH_CONFIRM, got %v", confirm, err)
		}
	}

	c, model, err := buildClassifier(config.LLMConfig{
		Mode: "hash", HashConfirm: "local-pilot-only", BaseURL: "https://x", APIKey: "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(llm.HashClassifier); !ok || model != "hash" {
		t.Fatalf("confirmed hash: got %T %q, want hash", c, model)
	}
}

func TestBuildClassifierRejectsUnknownMode(t *testing.T) {
	for _, mode := range []string{"HASH", "azure", "stub", " hash"} {
		_, _, err := buildClassifier(config.LLMConfig{Mode: mode, HashConfirm: "local-pilot-only"})
		if err == nil || !strings.Contains(err.Error(), "APP_LLM_MODE") {
			t.Fatalf("mode %q: want an error naming APP_LLM_MODE, got %v", mode, err)
		}
	}
}
