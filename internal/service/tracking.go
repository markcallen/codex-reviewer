package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	recordPath := filepath.Join(baseDir, "record.json")
	existing, _ := ReadReviewRecord(recordPath)
	reportPath := opts.ReportPath
	if reportPath == "" {
		reportPath = existing.ReportPath
	}
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
	request := opts.Request
	if reviewRequestIsZero(request) {
		request = existing.Request
	}
	apiURL := firstNonEmpty(opts.APIURL, existing.APIURL)
	response := mergeReviewResponse(existing.Response, opts.Response)
	record := ReviewRecord{
		SchemaVersion: "codex-reviewer.review_record.v1",
		ID:            response.ID,
		Status:        response.Status,
		Verdict:       firstNonEmpty(response.Verdict, verdictFromReport(opts.Report), existing.Verdict),
		Profile:       response.Profile,
		JobName:       response.JobName,
		APIURL:        apiURL,
		ReportURL:     response.ReportURL,
		ReportPath:    reportPath,
		Request:       request,
		Response:      response,
		RecordedAt:    opts.Now().UTC().Format(time.RFC3339),
	}
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

func ReadReviewRecord(path string) (ReviewRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReviewRecord{}, err
	}
	var record ReviewRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return ReviewRecord{}, err
	}
	return record, nil
}

func reviewRequestIsZero(req ReviewRequest) bool {
	return req.RepoURL == "" &&
		req.BaseRef == "" &&
		req.HeadRef == "" &&
		req.HeadSHA == "" &&
		req.ProfileName == "" &&
		req.Profile == (Profile{}) &&
		req.Instructions == "" &&
		len(req.Directives) == 0 &&
		len(req.Ignore) == 0 &&
		req.PolicyFile == "" &&
		req.ReturnFormat == ""
}

func mergeReviewResponse(existing, next ReviewResponse) ReviewResponse {
	if next.ID == "" {
		next.ID = existing.ID
	}
	if next.Status == "" {
		next.Status = existing.Status
	}
	if next.Verdict == "" {
		next.Verdict = existing.Verdict
	}
	if next.Profile == "" {
		next.Profile = existing.Profile
	}
	if next.JobName == "" {
		next.JobName = existing.JobName
	}
	if next.ReportURL == "" {
		next.ReportURL = existing.ReportURL
	}
	if next.Error == "" {
		next.Error = existing.Error
	}
	return next
}

func verdictFromReport(report []byte) string {
	if len(report) == 0 {
		return ""
	}
	if verdict := ParseVerdict(report); verdict != "unknown" {
		return verdict
	}
	return ""
}
