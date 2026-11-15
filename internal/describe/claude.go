package describe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// DefaultModel is used when VELOPIM_AI_MODEL is not set.
const DefaultModel = "claude-opus-5-5"

// Generator drafts descriptions with the Claude API.
type Generator struct {
	client anthropic.Client
	model  string
}

// FromEnv builds a generator from the environment. Without ANTHROPIC_API_KEY
// there is none and the feature is switched off.
func FromEnv(getenv func(string) string) (*Generator, bool) {
	key := getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, false
	}
	model := getenv("VELOPIM_AI_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return New(model, option.WithAPIKey(key)), true
}

// New builds a generator; tests pass a base URL of a fake server.
func New(model string, opts ...option.RequestOption) *Generator {
	opts = append([]option.RequestOption{option.WithRequestTimeout(3 * time.Minute)}, opts...)
	return &Generator{client: anthropic.NewClient(opts...), model: model}
}

// Model is the model the generator asks.
func (g *Generator) Model() string { return g.model }

// ErrRefused means the model declined to write the text.
var ErrRefused = errors.New("describe: the model declined to write this text")

// Generate asks the model for a description of the product.
func (g *Generator) Generate(ctx context.Context, in Input) (Draft, error) {
	resp, err := g.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     g.model,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: System}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(Prompt(in)))},
		// A short, factual text: little reasoning is needed. The schema
		// makes the answer machine readable without parsing prose.
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortLow,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: Schema},
		},
		// If a safety classifier declines the request by mistake, another
		// model answers within the same call.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	})
	if err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			switch apierr.StatusCode {
			case 401, 403:
				return Draft{}, fmt.Errorf("describe: the API key was not accepted (HTTP %d)", apierr.StatusCode)
			case 404:
				return Draft{}, fmt.Errorf("describe: model %q is not known to the API", g.model)
			case 429:
				return Draft{}, errors.New("describe: the API limit is reached, try again later")
			}
			return Draft{}, fmt.Errorf("describe: API error (HTTP %d, request %s)", apierr.StatusCode, apierr.RequestID)
		}
		return Draft{}, fmt.Errorf("describe: the API was not reached: %w", err)
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return Draft{}, ErrRefused
	case anthropic.BetaStopReasonMaxTokens:
		return Draft{}, errors.New("describe: the answer was cut off")
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	return Parse(text.String())
}
