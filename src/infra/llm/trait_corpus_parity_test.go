package llm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// traitCorpusSHA256 pins traitdata/trait-corpus.json. The frontend generates
// the file (scripts/traits/generate-trait-corpus.mts) and pins the same hash in
// scripts/traits/traitCorpusParity.test.ts; after regenerating, copy the file
// here and update both constants.
const traitCorpusSHA256 = "7daeba046d24cd4b8eb0f0aaf1e7c1811d5c328209b1fae745f3a386d8fe7e33"

func TestTraitCorpusIsTheVersionedCopy(t *testing.T) {
	sum := sha256.Sum256(traitCorpusJSON)
	if got := hex.EncodeToString(sum[:]); got != traitCorpusSHA256 {
		t.Fatalf("traitdata/trait-corpus.json sha256 %s; want %s (copy it from the frontend's scripts/traits/trait-corpus.json)", got, traitCorpusSHA256)
	}
}

// When the frontend repo sits next to this one, its copy must be the same bytes.
func TestTraitCorpusMatchesTheFrontendCopy(t *testing.T) {
	theirs, err := os.ReadFile("../../../../AI-Joke-Factory/scripts/traits/trait-corpus.json")
	if os.IsNotExist(err) {
		t.Skip("the frontend repo is not next to this one")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(traitCorpusJSON, theirs) {
		t.Fatal("traitdata/trait-corpus.json differs from the frontend's scripts/traits/trait-corpus.json")
	}
}
