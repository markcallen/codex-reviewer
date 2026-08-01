package usage

import "testing"

func TestEstimateTokensUsesPromptDiffAndChangedFiles(t *testing.T) {
	estimate := EstimateTokens(EstimateInput{
		Model:        "gpt-5.5",
		Prompt:       "review this branch",
		ChangedFiles: 3,
		DiffBytes:    4000,
	})
	if estimate.Model != "gpt-5.5" {
		t.Fatalf("Model = %q", estimate.Model)
	}
	if estimate.InputTokens.Min < 1480 {
		t.Fatalf("InputTokens.Min = %d, want prompt + diff + changed-file context", estimate.InputTokens.Min)
	}
	if estimate.InputTokens.Max <= estimate.InputTokens.Min {
		t.Fatalf("InputTokens.Max = %d, want greater than min %d", estimate.InputTokens.Max, estimate.InputTokens.Min)
	}
	if estimate.OutputTokens.Min == 0 || estimate.OutputTokens.Max == 0 {
		t.Fatalf("OutputTokens not populated: %#v", estimate.OutputTokens)
	}
}

func TestEstimateCostSplitsInputAndOutputRates(t *testing.T) {
	pricing := Pricing{Model: "gpt-5.5", InputPerMillionUSD: 5, OutputPerMillionUSD: 30}
	got := EstimateCost(TokenEstimate{
		Model:        "gpt-5.5",
		InputTokens:  TokenRange{Min: 1_000_000, Max: 2_000_000},
		OutputTokens: TokenRange{Min: 100_000, Max: 200_000},
	}, pricing)
	if got.CostUSD.MinUSD != 8 {
		t.Fatalf("min cost = %v, want 8", got.CostUSD.MinUSD)
	}
	if got.CostUSD.MaxUSD != 16 {
		t.Fatalf("max cost = %v, want 16", got.CostUSD.MaxUSD)
	}
}

func TestActualCostIncludesCachedInput(t *testing.T) {
	got := ActualCost(ActualTokenUsage{
		Status:            "available",
		InputTokens:       1_000_000,
		CachedInputTokens: 1_000_000,
		OutputTokens:      100_000,
	}, Pricing{InputPerMillionUSD: 5, CachedInputPerMillionUSD: 0.5, OutputPerMillionUSD: 30})
	if got != 8.5 {
		t.Fatalf("ActualCost() = %v, want 8.5", got)
	}
}

func TestFormatCostRange(t *testing.T) {
	got := FormatCostRange(CostRange{MinUSD: 0.0012, MaxUSD: 0.0456})
	if got != "$0.0012-$0.0456" {
		t.Fatalf("FormatCostRange() = %q", got)
	}
}

func TestParseActualUsageJSONLine(t *testing.T) {
	got, ok := ParseActualUsageJSONLine([]byte(`{"type":"usage","usage":{"input_tokens":12,"cached_input_tokens":3,"output_tokens":7}}`))
	if !ok {
		t.Fatal("ParseActualUsageJSONLine() ok = false")
	}
	if got.InputTokens != 12 || got.CachedInputTokens != 3 || got.OutputTokens != 7 || got.Status != "available" {
		t.Fatalf("usage = %#v", got)
	}
}

func TestParseActualUsageJSONLineSupportsAlternateNames(t *testing.T) {
	got, ok := ParseActualUsageJSONLine([]byte(`{"token_usage":{"prompt_tokens":12,"cached_tokens":3,"completion_tokens":7}}`))
	if !ok {
		t.Fatal("ParseActualUsageJSONLine() ok = false")
	}
	if got.InputTokens != 12 || got.CachedInputTokens != 3 || got.OutputTokens != 7 {
		t.Fatalf("usage = %#v", got)
	}
}
