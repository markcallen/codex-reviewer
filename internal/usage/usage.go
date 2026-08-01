package usage

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

type TokenRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type TokenEstimate struct {
	Model        string     `json:"model"`
	InputTokens  TokenRange `json:"input_tokens"`
	OutputTokens TokenRange `json:"output_tokens"`
	Notes        []string   `json:"notes,omitempty"`
}

type ActualTokenUsage struct {
	InputTokens       int    `json:"input_tokens,omitempty"`
	CachedInputTokens int    `json:"cached_input_tokens,omitempty"`
	OutputTokens      int    `json:"output_tokens,omitempty"`
	Status            string `json:"status"`
}

type Pricing struct {
	Model                    string  `json:"model"`
	InputPerMillionUSD       float64 `json:"input_per_million_usd"`
	CachedInputPerMillionUSD float64 `json:"cached_input_per_million_usd"`
	OutputPerMillionUSD      float64 `json:"output_per_million_usd"`
	Source                   string  `json:"source,omitempty"`
	AsOf                     string  `json:"as_of,omitempty"`
}

type CostRange struct {
	MinUSD float64 `json:"min_usd"`
	MaxUSD float64 `json:"max_usd"`
}

type CostEstimate struct {
	Model   string    `json:"model"`
	CostUSD CostRange `json:"cost_usd"`
	Pricing Pricing   `json:"pricing"`
}

type EstimateInput struct {
	Model              string
	Prompt             string
	ChangedFiles       int
	DiffBytes          int
	AdditionalContext  int
	OutputTokenMinimum int
	OutputTokenMaximum int
}

func EstimateTokens(input EstimateInput) TokenEstimate {
	promptTokens := ApproxTokens(input.Prompt)
	diffTokens := int(math.Ceil(float64(max(input.DiffBytes, 0)) / 4.0))
	contextTokens := max(input.AdditionalContext, 0)
	if input.ChangedFiles > 0 {
		contextTokens += input.ChangedFiles * 160
	}
	baseInput := promptTokens + diffTokens + contextTokens
	if baseInput < 500 {
		baseInput = 500
	}
	outMin := input.OutputTokenMinimum
	if outMin <= 0 {
		outMin = 800
	}
	outMax := input.OutputTokenMaximum
	if outMax < outMin {
		outMax = max(outMin*2, 1600)
	}
	notes := []string{"estimate is approximate and based on prompt, diff size, and changed-file count"}
	if input.ChangedFiles > 0 {
		notes = append(notes, fmt.Sprintf("%d changed files included", input.ChangedFiles))
	}
	return TokenEstimate{
		Model:        input.Model,
		InputTokens:  TokenRange{Min: baseInput, Max: int(math.Ceil(float64(baseInput) * 1.6))},
		OutputTokens: TokenRange{Min: outMin, Max: outMax},
		Notes:        notes,
	}
}

func ApproxTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(utf8.RuneCountInString(text)) / 4.0))
}

func EstimateCost(tokens TokenEstimate, pricing Pricing) CostEstimate {
	minCost := cost(tokens.InputTokens.Min, pricing.InputPerMillionUSD) + cost(tokens.OutputTokens.Min, pricing.OutputPerMillionUSD)
	maxCost := cost(tokens.InputTokens.Max, pricing.InputPerMillionUSD) + cost(tokens.OutputTokens.Max, pricing.OutputPerMillionUSD)
	return CostEstimate{
		Model:   tokens.Model,
		CostUSD: CostRange{MinUSD: roundUSD(minCost), MaxUSD: roundUSD(maxCost)},
		Pricing: pricing,
	}
}

func ActualCost(tokens ActualTokenUsage, pricing Pricing) float64 {
	if tokens.Status != "available" {
		return 0
	}
	total := cost(tokens.InputTokens, pricing.InputPerMillionUSD)
	total += cost(tokens.CachedInputTokens, pricing.CachedInputPerMillionUSD)
	total += cost(tokens.OutputTokens, pricing.OutputPerMillionUSD)
	return roundUSD(total)
}

func FormatCostRange(cost CostRange) string {
	if cost.MinUSD == cost.MaxUSD {
		return FormatUSD(cost.MinUSD)
	}
	return fmt.Sprintf("%s-%s", FormatUSD(cost.MinUSD), FormatUSD(cost.MaxUSD))
}

func FormatUSD(value float64) string {
	if value < 0.0001 && value > 0 {
		return "<$0.0001"
	}
	return fmt.Sprintf("$%.4f", value)
}

func DefaultPricing(model string) Pricing {
	return Pricing{
		Model:                    model,
		InputPerMillionUSD:       5,
		CachedInputPerMillionUSD: 0.5,
		OutputPerMillionUSD:      30,
		Source:                   "default codex-reviewer configurable estimate",
		AsOf:                     "2026-07-28",
	}
}

func ParseActualUsageJSONLine(line []byte) (ActualTokenUsage, bool) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return ActualTokenUsage{}, false
	}
	usageValue, ok := raw["usage"]
	if !ok {
		usageValue = raw["token_usage"]
	}
	usageMap, ok := usageValue.(map[string]any)
	if !ok {
		return ActualTokenUsage{}, false
	}
	actual := ActualTokenUsage{
		InputTokens:       intField(usageMap, "input_tokens", "prompt_tokens"),
		CachedInputTokens: intField(usageMap, "cached_input_tokens", "cached_tokens"),
		OutputTokens:      intField(usageMap, "output_tokens", "completion_tokens"),
		Status:            "available",
	}
	if actual.InputTokens == 0 && actual.CachedInputTokens == 0 && actual.OutputTokens == 0 {
		return ActualTokenUsage{}, false
	}
	return actual, true
}

func intField(values map[string]any, keys ...string) int {
	for _, key := range keys {
		switch v := values[key].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		}
	}
	return 0
}

func cost(tokens int, perMillion float64) float64 {
	return float64(tokens) * perMillion / 1_000_000
}

func roundUSD(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
