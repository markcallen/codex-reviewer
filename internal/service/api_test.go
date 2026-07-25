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

func (a *fakeApplier) Apply(_ context.Context, manifest []byte) error {
	a.manifest = append([]byte(nil), manifest...)
	return a.err
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
	if !strings.Contains(string(applier.manifest), `"kind": "Job"`) {
		t.Fatalf("applied manifest missing Job:\n%s", applier.manifest)
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
