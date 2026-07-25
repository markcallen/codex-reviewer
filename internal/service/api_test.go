package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeApplier struct {
	manifest []byte
	err      error
}

type fakeReportReader struct {
	report []byte
	err    error
}

func (a *fakeApplier) Apply(_ context.Context, manifest []byte) error {
	a.manifest = append([]byte(nil), manifest...)
	return a.err
}

func (r fakeReportReader) ReadReport(_ context.Context, _, _ string) ([]byte, error) {
	return r.report, r.err
}

func TestAPIServerCreatesReviewJob(t *testing.T) {
	applier := &fakeApplier{}
	server, err := NewAPIServer(APIOptions{
		JobOptions: JobOptions{
			ReviewerImage:    "reviewer:test",
			SidecarImage:     "sidecar:test",
			OpenAISecretName: "openai-api",
		},
		Applier: applier,
		Reports: fakeReportReader{report: []byte("No blocking findings\n")},
	})
	if err != nil {
		t.Fatalf("NewAPIServer() error = %v", err)
	}

	req := testReviewRequest(t)
	body, err := req.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader(string(body)))
	server.Handler().ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp ReviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response error = %v", err)
	}
	if resp.ID == "" || resp.Status != "submitted" || resp.Profile != "standard" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp.JobName == "" {
		t.Fatalf("response missing job name: %#v", resp)
	}
	if !strings.Contains(string(applier.manifest), `"kind": "Job"`) {
		t.Fatalf("applied manifest missing Job:\n%s", applier.manifest)
	}

	reportReq := httptest.NewRequest(http.MethodGet, resp.ReportURL, nil)
	reportRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reportRec, reportReq)
	if reportRec.Code != http.StatusOK {
		t.Fatalf("report status = %d, body = %s", reportRec.Code, reportRec.Body.String())
	}
	if reportRec.Body.String() != "No blocking findings\n" {
		t.Fatalf("report body = %q", reportRec.Body.String())
	}
}

func TestClientSubmitsReview(t *testing.T) {
	req := testReviewRequest(t)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/reviews" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusAccepted, ReviewResponse{ID: "review-1", Status: "submitted", Profile: "standard"})
	}))
	defer httpServer.Close()

	resp, err := Client{BaseURL: httpServer.URL}.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if resp.ID != "review-1" || resp.Status != "submitted" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestClientReadsReport(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/reviews/review-1/report" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("Block\n"))
	}))
	defer httpServer.Close()

	report, err := Client{BaseURL: httpServer.URL}.Report(context.Background(), "/reviews/review-1/report")
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if string(report) != "Block\n" {
		t.Fatalf("report = %q", report)
	}
}
