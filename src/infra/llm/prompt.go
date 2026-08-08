package llm

import (
	"fmt"
	"strings"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

const (
	systemPrompt = `You are a precise joke classifier. You will be given one or more jokes (each with a title)
and a set of dimensions, each with an allowed list of categories. For EACH joke and EACH dimension,
choose EXACTLY ONE category from its allowed list that best describes the joke.
If none of the normal categories fit, choose "None of the above" for that dimension.
Do not invent categories. Respond ONLY with JSON matching the provided schema.

Judge "title_fit" as how well the title matches the joke's actual theme/subject.`

	jsonSchemaType = "type"
)

// jsonKey maps domain dimensions to the structured-output property names.
var jsonKey = map[domain.Dimension]string{
	domain.DimTopic:       "topic",
	domain.DimHumorStyle:  "humor_style",
	domain.DimComplexity:  "complexity",
	domain.DimEdginess:    "edginess",
	domain.DimStructure:   "structure",
	domain.DimWordplay:    "wordplay",
	domain.DimFreshness:   "freshness",
	domain.DimSetupPayoff: "setup_payoff",
	domain.DimClarity:     "clarity",
	domain.DimEnergy:      "energy",
	domain.DimTitleFit:    "title_fit",
}

func buildUserPrompt(jokes []ports.JokeInput) string {
	var b strings.Builder
	b.WriteString("Classify each joke below.\n\nAllowed categories per dimension:\n")
	for _, dim := range scoring.LLMDimensions() {
		spec := scoring.Specs[dim]
		_, _ = fmt.Fprintf(&b, "- %s (%s): [%s]\n",
			jsonKey[dim], dim, strings.Join(spec.Categories, ", "))
	}
	b.WriteString("\nJokes:\n")
	for i, j := range jokes {
		_, _ = fmt.Fprintf(&b, "%d) joke_id=%d\n   title: %q\n   text: %q\n",
			i+1, j.JokeID, j.Title, j.Text)
	}
	return b.String()
}

// batchResponseSchema is the JSON Schema for a multi-joke classification reply.
func batchResponseSchema() map[string]any {
	jokeProps := map[string]any{
		"joke_id": map[string]any{jsonSchemaType: "integer"},
	}
	required := make([]string, 0, 1+len(scoring.LLMDimensions()))
	required = append(required, "joke_id")
	for _, dim := range scoring.LLMDimensions() {
		key := jsonKey[dim]
		jokeProps[key] = map[string]any{
			jsonSchemaType: "string",
			"enum":         scoring.Specs[dim].Categories,
		}
		required = append(required, key)
	}
	return map[string]any{
		jsonSchemaType: "object",
		"properties": map[string]any{
			"jokes": map[string]any{
				jsonSchemaType: "array",
				"items": map[string]any{
					jsonSchemaType:         "object",
					"properties":           jokeProps,
					"required":             required,
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"jokes"},
		"additionalProperties": false,
	}
}
