package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
	"jokefactory/src/infra/config"
)

// AzureClassifier calls Azure AI Foundry via the OpenAI-compatible /openai/v1/ API.
type AzureClassifier struct {
	client      openai.Client
	deployment  string
	temperature float64
	maxRetries  int
}

// NewAzureClassifier builds a Classifier for the configured Foundry deployment.
func NewAzureClassifier(cfg config.LLMConfig) *AzureClassifier {
	retries := cfg.MaxRetries
	if retries < 0 {
		retries = 0
	}
	client := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(cfg.APIKey),
	)
	return &AzureClassifier{
		client:      client,
		deployment:  cfg.Deployment,
		temperature: cfg.Temperature,
		maxRetries:  retries,
	}
}

type batchLLMResponse struct {
	Jokes []map[string]any `json:"jokes"`
}

// Classify implements ports.Classifier with structured JSON output + retries.
func (c *AzureClassifier) Classify(ctx context.Context, jokes []ports.JokeInput) ([]ports.JokeClassification, error) {
	if len(jokes) == 0 {
		return nil, nil
	}

	var lastErr error
	attempts := c.maxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		out, err := c.classifyOnce(ctx, jokes)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("classify after %d attempts: %w", attempts, lastErr)
}

func (c *AzureClassifier) classifyOnce(ctx context.Context, jokes []ports.JokeInput) ([]ports.JokeClassification, error) {
	params := openai.ChatCompletionNewParams{
		Model: c.deployment,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(buildUserPrompt(jokes)),
		},
		Temperature: openai.Float(c.temperature),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "joke_batch_classification",
					Strict: openai.Bool(true),
					Schema: batchResponseSchema(),
				},
			},
		},
	}

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai chat completion: empty choices")
	}
	raw := resp.Choices[0].Message.Content
	if raw == "" {
		return nil, fmt.Errorf("openai chat completion: empty content")
	}

	var parsed batchLLMResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse classification json: %w", err)
	}
	return validateBatchResponse(jokes, parsed)
}

func validateBatchResponse(jokes []ports.JokeInput, parsed batchLLMResponse) ([]ports.JokeClassification, error) {
	byID := make(map[int64]map[string]any, len(parsed.Jokes))
	for _, row := range parsed.Jokes {
		id, err := asInt64(row["joke_id"])
		if err != nil {
			return nil, fmt.Errorf("invalid joke_id in LLM response: %w", err)
		}
		byID[id] = row
	}

	out := make([]ports.JokeClassification, 0, len(jokes))
	for _, j := range jokes {
		row, ok := byID[j.JokeID]
		if !ok {
			return nil, fmt.Errorf("LLM response missing joke_id %d", j.JokeID)
		}
		cats := make(map[domain.Dimension]string, len(scoring.LLMDimensions()))
		for _, dim := range scoring.LLMDimensions() {
			key := jsonKey[dim]
			raw, ok := row[key]
			if !ok {
				return nil, fmt.Errorf("LLM response missing %s for joke %d", key, j.JokeID)
			}
			cat, ok := raw.(string)
			if !ok || cat == "" {
				return nil, fmt.Errorf("LLM response invalid %s for joke %d", key, j.JokeID)
			}
			if !scoring.IsValidCategory(dim, cat) {
				return nil, fmt.Errorf("LLM returned invalid category %q for %s (joke %d)", cat, dim, j.JokeID)
			}
			cats[dim] = cat
		}
		out = append(out, ports.JokeClassification{JokeID: j.JokeID, Categories: cats})
	}
	return out, nil
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case json.Number:
		return n.Int64()
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

func backoff(attempt int) time.Duration {
	// attempt is 1-based when called (after first failure).
	d := time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	return d
}
