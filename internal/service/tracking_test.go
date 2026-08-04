package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrackReviewWritesRecordAndReport(t *testing.T) {
	dir := t.TempDir()
	req := testReviewRequest(t)
	recordPath, err := TrackReview(TrackReviewOptions{
		Dir:     dir,
		APIURL:  "http://127.0.0.1:8080",
		Request: req,
		Response: ReviewResponse{
			ID:        "review-123",
			Status:    "submitted",
			Profile:   "standard",
			JobName:   "codex-review-review-123",
			ReportURL: "/reviews/review-123/report",
		},
		Report: []byte("  # No blocking findings (checked auth)\n\nChecked auth.\n"),
		Now:    func() time.Time { return time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("TrackReview() error = %v", err)
	}
	wantPath := filepath.Join(dir, "codex-review", "k8s-reviews", "review-123", "record.json")
	if recordPath != wantPath {
		t.Fatalf("recordPath = %q, want %q", recordPath, wantPath)
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	var record ReviewRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(record) error = %v", err)
	}
	if record.Verdict != "no_blocking_findings" || record.Request.HeadSHA != req.HeadSHA {
		t.Fatalf("record = %#v", record)
	}
	report, err := os.ReadFile(filepath.Join(dir, "codex-review", "k8s-reviews", "review-123", "review.md"))
	if err != nil {
		t.Fatalf("ReadFile(report) error = %v", err)
	}
	if !strings.Contains(string(report), "Checked auth.") {
		t.Fatalf("report = %q", report)
	}
	if strings.Contains(string(data), "CODEX_AUTH") || strings.Contains(string(data), "api_key") {
		t.Fatalf("record contains secret-looking auth fields:\n%s", data)
	}
}

func TestTrackReviewWritesRelativeReportPathUnderDir(t *testing.T) {
	dir := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	reportPath := filepath.Join("codex-review", "custom.md")
	recordPath, err := TrackReview(TrackReviewOptions{
		Dir: dir,
		Response: ReviewResponse{
			ID:      "review-relative",
			Status:  "succeeded",
			Profile: "standard",
		},
		Report:     []byte("Block\n\n- P1 example\n"),
		ReportPath: reportPath,
	})
	if err != nil {
		t.Fatalf("TrackReview() error = %v", err)
	}
	if recordPath != filepath.Join(dir, "codex-review", "k8s-reviews", "review-relative", "record.json") {
		t.Fatalf("recordPath = %q", recordPath)
	}
	if _, err := os.Stat(filepath.Join(dir, reportPath)); err != nil {
		t.Fatalf("tracked report under Dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, reportPath)); !os.IsNotExist(err) {
		t.Fatalf("tracked report should not be written under cwd, stat err = %v", err)
	}

	record := readReviewRecordForTest(t, recordPath)
	if record.ReportPath != reportPath || record.Verdict != "block" {
		t.Fatalf("record = %#v", record)
	}
}

func readReviewRecordForTest(t *testing.T, path string) ReviewRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	var record ReviewRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(record) error = %v", err)
	}
	return record
}
