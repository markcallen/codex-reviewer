package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
)

type APIOptions struct {
	JobOptions JobOptions
	Applier    JobApplier
	Reports    ReportReader
	Telemetry  TelemetryRecorder
}

type JobApplier interface {
	Apply(ctx context.Context, manifest []byte) error
}

type ReportReader interface {
	ReadReport(ctx context.Context, namespace, jobName string) ([]byte, error)
}

type TelemetryRecorder interface {
	Ingest(event ReviewTelemetryEvent) (ReviewTelemetryEvent, error)
}

type KubectlApplier struct{}

type KubectlReportReader struct{}

type ReviewResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Verdict   string `json:"verdict,omitempty"`
	Profile   string `json:"profile"`
	JobName   string `json:"job_name,omitempty"`
	ReportURL string `json:"report_url,omitempty"`
	Error     string `json:"error,omitempty"`
}

type APIServer struct {
	opts    APIOptions
	mu      sync.Mutex
	reviews map[string]ReviewResponse
}

func NewAPIServer(opts APIOptions) (*APIServer, error) {
	if opts.Applier == nil {
		opts.Applier = KubectlApplier{}
	}
	if opts.Reports == nil {
		opts.Reports = KubectlReportReader{}
	}
	if opts.JobOptions.ReviewerImage == "" {
		return nil, fmt.Errorf("reviewer image is required")
	}
	if opts.JobOptions.SidecarImage == "" {
		return nil, fmt.Errorf("sidecar image is required")
	}
	if opts.JobOptions.OpenAISecretName == "" {
		return nil, fmt.Errorf("OpenAI secret name is required")
	}
	return &APIServer{opts: opts, reviews: make(map[string]ReviewResponse)}, nil
}

func (s *APIServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("POST /reviews", s.handleCreateReview)
	mux.HandleFunc("GET /reviews/{id}", s.handleGetReview)
	mux.HandleFunc("GET /reviews/{id}/report", s.handleGetReport)
	return mux
}

func (s *APIServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *APIServer) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *APIServer) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var req ReviewRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ReviewResponse{Status: "rejected", Error: "decode request: " + err.Error()})
		return
	}
	if req.ProfileName == "" {
		req.ProfileName = "standard"
	}
	if req.Profile.Name == "" {
		profile, err := ResolveProfile(req.ProfileName)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ReviewResponse{Status: "rejected", Error: err.Error()})
			return
		}
		req.Profile = profile
	}
	id := newReviewID()
	jobOpts := s.opts.JobOptions
	jobOpts.ReviewID = id
	manifest, err := JobManifest(req, jobOpts)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ReviewResponse{Status: "rejected", Error: err.Error()})
		return
	}
	resp := ReviewResponse{
		ID:        id,
		Status:    "submitted",
		Profile:   req.ProfileName,
		JobName:   JobName(id),
		ReportURL: "/reviews/" + id + "/report",
	}
	s.mu.Lock()
	s.reviews[id] = resp
	s.mu.Unlock()
	if err := s.opts.Applier.Apply(r.Context(), manifest); err != nil {
		resp.Status = "failed"
		resp.Error = err.Error()
		s.mu.Lock()
		s.reviews[id] = resp
		s.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, resp)
		return
	}
	if s.opts.Telemetry != nil {
		event := ReviewTelemetryEvent{
			SchemaVersion: ReviewTelemetryEventSchemaV1,
			ReviewID:      id,
			Status:        "submitted",
			Profile:       req.ProfileName,
			Model:         req.Profile.Model,
			BaseRef:       req.BaseRef,
			HeadRef:       req.HeadRef,
			HeadSHA:       req.HeadSHA,
		}
		go func() { _, _ = s.opts.Telemetry.Ingest(event) }()
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *APIServer) handleGetReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, ok := s.review(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, ReviewResponse{ID: id, Status: "missing"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *APIServer) handleGetReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.review(id); !ok {
		http.NotFound(w, r)
		return
	}
	report, err := s.opts.Reports.ReadReport(r.Context(), s.opts.JobOptions.Namespace, JobName(id))
	if err != nil {
		http.Error(w, "review report is not available yet: "+err.Error(), http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write(report)
}

func (s *APIServer) review(id string) (ReviewResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp, ok := s.reviews[id]
	return resp, ok
}

func (KubectlApplier) Apply(ctx context.Context, manifest []byte) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(manifest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
}

func (KubectlReportReader) ReadReport(ctx context.Context, namespace, jobName string) ([]byte, error) {
	args := []string{"logs", "job/" + jobName, "-c", "reviewer"}
	if namespace != "" {
		args = append([]string{"-n", namespace}, args...)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return out, nil
}

func newReviewID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "review"
	}
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
