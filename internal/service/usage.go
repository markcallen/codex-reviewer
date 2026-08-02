package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/markcallen/codex-reviewer/internal/usage"
)

type ReviewUsageEstimate struct {
	TokenEstimate usage.TokenEstimate `json:"token_estimate"`
	CostEstimate  usage.CostEstimate  `json:"cost_estimate"`
}

func EstimateReviewUsage(ctx context.Context, dir string, req ReviewRequest) ReviewUsageEstimate {
	if dir == "" {
		dir = "."
	}
	requestJSON, _ := req.JSON()
	diffBytes := len(requestJSON)
	changedFiles := 0
	if req.BaseRef != "" && req.HeadRef != "" {
		if diff, err := gitOutput(ctx, dir, "diff", "--no-ext-diff", "--binary", req.BaseRef+"..."+req.HeadRef); err == nil && diff != "" {
			diffBytes = len(diff)
		}
		if names, err := gitOutput(ctx, dir, "diff", "--name-only", req.BaseRef+"..."+req.HeadRef); err == nil {
			changedFiles = countNonEmptyLines(names)
		}
	}
	prompt := strings.Join([]string{
		req.Profile.Agent,
		req.Profile.Prompt,
		req.Profile.ReasoningEffort,
		req.Instructions,
		strings.Join(req.Directives, "\n"),
		strings.Join(req.Ignore, "\n"),
		req.PolicyFile,
		req.ReturnFormat,
	}, "\n")
	tokens := usage.EstimateTokens(usage.EstimateInput{
		Model:        req.Profile.Model,
		Prompt:       prompt,
		ChangedFiles: changedFiles,
		DiffBytes:    diffBytes,
	})
	pricing := usage.DefaultPricing(req.Profile.Model)
	return ReviewUsageEstimate{
		TokenEstimate: tokens,
		CostEstimate:  usage.EstimateCost(tokens, pricing),
	}
}

func DryRunReviewRequestJSON(ctx context.Context, dir string, req ReviewRequest) ([]byte, error) {
	type dryRunReviewRequest struct {
		ReviewRequest
		UsageEstimate ReviewUsageEstimate `json:"usage_estimate"`
	}
	data, err := json.MarshalIndent(dryRunReviewRequest{
		ReviewRequest: req,
		UsageEstimate: EstimateReviewUsage(ctx, dir, req),
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func countNonEmptyLines(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
