package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ReviewRecord struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	Verdict       string         `json:"verdict,omitempty"`
	Profile       string         `json:"profile"`
	JobName       string         `json:"job_name,omitempty"`
	APIURL        string         `json:"api_url,omitempty"`
	ReportURL     string         `json:"report_url,omitempty"`
	ReportPath    string         `json:"report_path,omitempty"`
	Request       ReviewRequest  `json:"request"`
	Response      ReviewResponse `json:"response"`
	RecordedAt    string         `json:"recorded_at"`
}

type TrackReviewOptions struct {
	Dir        string
	APIURL     string
	Request    ReviewRequest
	Response   ReviewResponse
	Report     []byte
	ReportPath string
	Now        func() time.Time
}

func TrackReview(opts TrackReviewOptions) (string, error) {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Response.ID == "" {
		return "", fmt.Errorf("review response id is required")
	}
	baseDir := filepath.Join(opts.Dir, "codex-review", "k8s-reviews", opts.Response.ID)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create review tracking directory: %w", err)
	}
	reportPath := opts.ReportPath
	if len(opts.Report) > 0 {
		if reportPath == "" {
			reportPath = filepath.Join(baseDir, "review.md")
		}
		if dir := filepath.Dir(reportPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", fmt.Errorf("create tracked report directory: %w", err)
			}
		}
		if err := os.WriteFile(reportPath, opts.Report, 0o644); err != nil {
			return "", fmt.Errorf("write tracked review report: %w", err)
		}
	}
	record := ReviewRecord{
		SchemaVersion: "codex-reviewer.review_record.v1",
		ID:            opts.Response.ID,
		Status:        opts.Response.Status,
		Verdict:       firstNonEmpty(opts.Response.Verdict, verdictFromReport(opts.Report)),
		Profile:       opts.Response.Profile,
		JobName:       opts.Response.JobName,
		APIURL:        opts.APIURL,
		ReportURL:     opts.Response.ReportURL,
		ReportPath:    reportPath,
		Request:       opts.Request,
		Response:      opts.Response,
		RecordedAt:    opts.Now().UTC().Format(time.RFC3339),
	}
	recordPath := filepath.Join(baseDir, "record.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(recordPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write review record: %w", err)
	}
	return recordPath, nil
}

func verdictFromReport(report []byte) string {
	firstLine := ""
	for _, line := range strings.Split(string(report), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" {
			firstLine = line
			break
		}
	}
	switch strings.ToLower(firstLine) {
	case "block":
		return "block"
	case "approve with fixes":
		return "approve_with_fixes"
	case "no blocking findings":
		return "no_blocking_findings"
	default:
		return ""
	}
}
