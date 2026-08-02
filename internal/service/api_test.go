package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeApplier struct {
	manifest []byte
	err      error
}

type fakeReportReader struct {
	report []byte
	err    error
}

type fakeTelemetryRecorder struct {
	events chan ReviewTelemetryEvent
}

func (a *fakeApplier) Apply(_ context.Context, manifest []byte) error {
	a.manifest = append([]byte(nil), manifest...)
	return a.err
}

func (r fakeReportReader) ReadReport(_ context.Context, _, _ string) ([]byte, error) {
	return r.report, r.err
}

func (r fakeTelemetryRecorder) Ingest(event ReviewTelemetryEvent) (ReviewTelemetryEvent, error) {
	r.events <- event
	return event, nil
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

func TestAPIServerRecordsSubmittedTelemetryWithoutBlocking(t *testing.T) {
	recorder := fakeTelemetryRecorder{events: make(chan ReviewTelemetryEvent, 1)}
	server, err := NewAPIServer(APIOptions{
		JobOptions: JobOptions{
			ReviewerImage:    "reviewer:test",
			SidecarImage:     "sidecar:test",
			OpenAISecretName: "openai-api",
		},
		Applier:   &fakeApplier{},
		Telemetry: recorder,
	})
	if err != nil {
		t.Fatalf("NewAPIServer() error = %v", err)
	}
	body, err := testReviewRequest(t).JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader(string(body))))
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	select {
	case event := <-recorder.events:
		if event.Status != "submitted" || event.Profile != "standard" || event.Model == "" {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry event")
	}
}

func TestAPIServerHealthAndReadiness(t *testing.T) {
	server, err := NewAPIServer(APIOptions{
		JobOptions: JobOptions{
			ReviewerImage:    "reviewer:test",
			SidecarImage:     "sidecar:test",
			OpenAISecretName: "openai-api",
		},
	})
	if err != nil {
		t.Fatalf("NewAPIServer() error = %v", err)
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/healthz", want: `"status":"ok"`},
		{path: "/readyz", want: `"status":"ready"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("body = %s, want %s", rec.Body.String(), tc.want)
			}
		})
	}
}

func TestAPIServerGetsSubmittedReview(t *testing.T) {
	server, err := NewAPIServer(APIOptions{
		JobOptions: JobOptions{
			ReviewerImage:    "reviewer:test",
			SidecarImage:     "sidecar:test",
			OpenAISecretName: "openai-api",
		},
		Applier: &fakeApplier{},
		Reports: fakeReportReader{report: []byte("No blocking findings\n")},
	})
	if err != nil {
		t.Fatalf("NewAPIServer() error = %v", err)
	}

	body, err := testReviewRequest(t).JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader(string(body))))
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created ReviewResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal response error = %v", err)
	}

	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/reviews/"+created.ID, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var got ReviewResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal get response error = %v", err)
	}
	if got.ID != created.ID || got.JobName != created.JobName {
		t.Fatalf("GET response = %#v, created = %#v", got, created)
	}
}

func TestAPIServerReportsApplyFailure(t *testing.T) {
	server, err := NewAPIServer(APIOptions{
		JobOptions: JobOptions{
			ReviewerImage:    "reviewer:test",
			SidecarImage:     "sidecar:test",
			OpenAISecretName: "openai-api",
		},
		Applier: &fakeApplier{err: errors.New("apply failed")},
	})
	if err != nil {
		t.Fatalf("NewAPIServer() error = %v", err)
	}
	body, err := testReviewRequest(t).JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader(string(body))))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp ReviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response error = %v", err)
	}
	if resp.Status != "failed" || !strings.Contains(resp.Error, "apply failed") {
		t.Fatalf("response = %#v", resp)
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

func TestClientGetsReviewStatus(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/reviews/review-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, ReviewResponse{ID: "review-1", Status: "submitted", Profile: "standard"})
	}))
	defer httpServer.Close()

	resp, err := Client{BaseURL: httpServer.URL}.Status(context.Background(), "review-1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
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

func TestClientWaitReportRetriesUntilAvailable(t *testing.T) {
	attempts := 0
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "not ready", http.StatusAccepted)
			return
		}
		_, _ = w.Write([]byte("No blocking findings\n"))
	}))
	defer httpServer.Close()

	report, err := Client{BaseURL: httpServer.URL}.WaitReport(context.Background(), "/reviews/review-1/report", time.Millisecond)
	if err != nil {
		t.Fatalf("WaitReport() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if string(report) != "No blocking findings\n" {
		t.Fatalf("report = %q", report)
	}
}

func TestKubectlApplierAndReportReader(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "kubectl.log")
	kubectlPath := filepath.Join(binDir, "kubectl")
	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
for arg in "$@"; do
  if [ "$arg" = "logs" ]; then
    printf 'No blocking findings\n'
    exit 0
  fi
done
`
	if err := os.WriteFile(kubectlPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := (KubectlApplier{}).Apply(context.Background(), []byte(`{"kind":"Job"}`)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	report, err := (KubectlReportReader{}).ReadReport(context.Background(), "reviews", "job-1")
	if err != nil {
		t.Fatalf("ReadReport() error = %v", err)
	}
	if string(report) != "No blocking findings\n" {
		t.Fatalf("report = %q", report)
	}
}
