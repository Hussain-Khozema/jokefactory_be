package scoring

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"testing"

	"jokefactory/src/core/domain"
)

// fitFixturesSHA256 pins testdata/fit-fixtures.json, which the frontend keeps
// byte-identical (config/fit-fixtures.json, checked by its
// scripts/traits/traitCorpusParity.test.ts) and runs against its TypeScript
// port of DimFit and TrueFit (config/fitFixtures.test.ts).
const fitFixturesSHA256 = "8ebe339deb91d1b661d6f4b1c25f2aab8d67da5ff4e73f303aee3bb55c431dd9"

type fitFixtures struct {
	Ideals map[string]map[domain.Dimension]string `json:"ideals"`
	Cases  []struct {
		Name           string                       `json:"name"`
		Ideal          string                       `json:"ideal"`
		Classification map[domain.Dimension]string  `json:"classification"`
		DimFit         map[domain.Dimension]float64 `json:"dim_fit"`
		TrueFit        float64                      `json:"true_fit"`
	} `json:"cases"`
}

func TestSharedFitFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/fit-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != fitFixturesSHA256 {
		t.Fatalf("testdata/fit-fixtures.json sha256 %x; want %s (copy it from the frontend's config/fit-fixtures.json)", sum, fitFixturesSHA256)
	}
	var fx fitFixtures
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) < 6 {
		t.Fatalf("%d cases; want at least 6", len(fx.Cases))
	}
	for _, c := range fx.Cases {
		t.Run(c.Name, func(t *testing.T) {
			ideal, ok := fx.Ideals[c.Ideal]
			if !ok {
				t.Fatalf("unknown ideal %q", c.Ideal)
			}
			if len(c.DimFit) != len(AllDimensions) {
				t.Fatalf("dim_fit lists %d dimensions; want %d", len(c.DimFit), len(AllDimensions))
			}
			for _, dim := range AllDimensions {
				if got := DimFit(dim, ideal[dim], c.Classification[dim]); got != c.DimFit[dim] {
					t.Errorf("DimFit(%s) = %v; want %v", dim, got, c.DimFit[dim])
				}
			}
			if got := TrueFit(Classification(c.Classification), IdealProfile(ideal)); math.Abs(got-c.TrueFit) > 1e-9 {
				t.Errorf("TrueFit = %v; want %v", got, c.TrueFit)
			}
		})
	}
}

// When the frontend repo sits next to this one, its copy must be the same bytes.
func TestSharedFitFixturesMatchTheFrontendCopy(t *testing.T) {
	theirs, err := os.ReadFile("../../../../../AI-Joke-Factory/config/fit-fixtures.json")
	if os.IsNotExist(err) {
		t.Skip("the frontend repo is not next to this one")
	}
	if err != nil {
		t.Fatal(err)
	}
	mine, err := os.ReadFile("testdata/fit-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mine, theirs) {
		t.Fatal("testdata/fit-fixtures.json differs from the frontend's config/fit-fixtures.json")
	}
}
