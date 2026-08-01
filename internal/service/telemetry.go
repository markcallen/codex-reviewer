package service

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/markcallen/codex-reviewer/internal/usage"
)

const ReviewTelemetryEventSchemaV1 = "codex-reviewer.review_telemetry.v1"

type ReviewTelemetryEvent struct {
	SchemaVersion string                 `json:"schema_version"`
	ReviewID      string                 `json:"review_id"`
	Status        string                 `json:"status"`
	Verdict       string                 `json:"verdict,omitempty"`
	Profile       string                 `json:"profile"`
	Model         string                 `json:"model"`
	BaseRef       string                 `json:"base_ref,omitempty"`
	HeadRef       string                 `json:"head_ref,omitempty"`
	HeadSHA       string                 `json:"head_sha,omitempty"`
	TokenUsage    usage.ActualTokenUsage `json:"token_usage"`
	CostUSD       float64                `json:"cost_usd,omitempty"`
	StartedAt     string                 `json:"started_at,omitempty"`
	FinishedAt    string                 `json:"finished_at,omitempty"`
	RecordedAt    string                 `json:"recorded_at"`
}

type TelemetryOptions struct {
	Token        string
	MaxBodyBytes int64
	Now          func() time.Time
}

type TelemetryServer struct {
	opts   TelemetryOptions
	mu     sync.Mutex
	events []ReviewTelemetryEvent
}

type SpendRollup struct {
	Events         int                `json:"events"`
	AvailableUsage int                `json:"available_usage"`
	TotalCostUSD   float64            `json:"total_cost_usd"`
	ByModel        map[string]float64 `json:"by_model"`
	ByProfile      map[string]float64 `json:"by_profile"`
}

type EffectivenessRollup struct {
	Events    int            `json:"events"`
	ByStatus  map[string]int `json:"by_status"`
	ByVerdict map[string]int `json:"by_verdict"`
	ByProfile map[string]int `json:"by_profile"`
}

func NewTelemetryServer(opts TelemetryOptions) (*TelemetryServer, error) {
	if strings.TrimSpace(opts.Token) == "" {
		return nil, fmt.Errorf("telemetry token is required")
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 256 << 10
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &TelemetryServer{opts: opts}, nil
}

func (s *TelemetryServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("POST /telemetry/v1/events", s.handleIngest)
	mux.HandleFunc("GET /telemetry/v1/spend", s.handleSpend)
	mux.HandleFunc("GET /telemetry/v1/effectiveness", s.handleEffectiveness)
	mux.HandleFunc("GET /telemetry/v1/export", s.handleExport)
	return mux
}

func (s *TelemetryServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *TelemetryServer) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *TelemetryServer) handleIngest(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.opts.MaxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload too large"})
		return
	}
	if containsRawTelemetryField(body) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "raw source, diff, prompt, and log payloads are not accepted"})
		return
	}
	var event ReviewTelemetryEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decode event: " + err.Error()})
		return
	}
	event = s.sanitizeEvent(event)
	if event.ReviewID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "review_id is required"})
		return
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	writeJSON(w, http.StatusAccepted, event)
}

func (s *TelemetryServer) handleSpend(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, s.SpendRollup())
}

func (s *TelemetryServer) handleEffectiveness(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, s.EffectivenessRollup())
}

func (s *TelemetryServer) handleExport(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, s.Events())
}

func (s *TelemetryServer) Ingest(event ReviewTelemetryEvent) (ReviewTelemetryEvent, error) {
	event = s.sanitizeEvent(event)
	if event.ReviewID == "" {
		return ReviewTelemetryEvent{}, fmt.Errorf("review_id is required")
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return event, nil
}

func (s *TelemetryServer) Events() []ReviewTelemetryEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ReviewTelemetryEvent(nil), s.events...)
}

func (s *TelemetryServer) SpendRollup() SpendRollup {
	rollup := SpendRollup{ByModel: map[string]float64{}, ByProfile: map[string]float64{}}
	for _, event := range s.Events() {
		rollup.Events++
		if event.TokenUsage.Status == "available" {
			rollup.AvailableUsage++
		}
		rollup.TotalCostUSD += event.CostUSD
		if event.Model != "" {
			rollup.ByModel[event.Model] += event.CostUSD
		}
		if event.Profile != "" {
			rollup.ByProfile[event.Profile] += event.CostUSD
		}
	}
	rollup.TotalCostUSD = roundTelemetryUSD(rollup.TotalCostUSD)
	for key, value := range rollup.ByModel {
		rollup.ByModel[key] = roundTelemetryUSD(value)
	}
	for key, value := range rollup.ByProfile {
		rollup.ByProfile[key] = roundTelemetryUSD(value)
	}
	return rollup
}

func (s *TelemetryServer) EffectivenessRollup() EffectivenessRollup {
	rollup := EffectivenessRollup{ByStatus: map[string]int{}, ByVerdict: map[string]int{}, ByProfile: map[string]int{}}
	for _, event := range s.Events() {
		rollup.Events++
		rollup.ByStatus[event.Status]++
		if event.Verdict != "" {
			rollup.ByVerdict[event.Verdict]++
		}
		if event.Profile != "" {
			rollup.ByProfile[event.Profile]++
		}
	}
	return rollup
}

func (s *TelemetryServer) sanitizeEvent(event ReviewTelemetryEvent) ReviewTelemetryEvent {
	if event.SchemaVersion == "" {
		event.SchemaVersion = ReviewTelemetryEventSchemaV1
	}
	event.ReviewID = redactDebugText(strings.TrimSpace(event.ReviewID))
	event.Status = redactDebugText(strings.TrimSpace(event.Status))
	event.Verdict = redactDebugText(strings.TrimSpace(event.Verdict))
	event.Profile = redactDebugText(strings.TrimSpace(event.Profile))
	event.Model = redactDebugText(strings.TrimSpace(event.Model))
	event.BaseRef = redactDebugText(strings.TrimSpace(event.BaseRef))
	event.HeadRef = redactDebugText(strings.TrimSpace(event.HeadRef))
	event.HeadSHA = redactDebugText(strings.TrimSpace(event.HeadSHA))
	if event.RecordedAt == "" {
		event.RecordedAt = s.opts.Now().UTC().Format(time.RFC3339)
	}
	if event.TokenUsage.Status == "" {
		event.TokenUsage.Status = "unavailable"
	}
	return event
}

func (s *TelemetryServer) authorized(r *http.Request) bool {
	token := strings.TrimSpace(s.opts.Token)
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") && strings.TrimSpace(auth[len("bearer "):]) == token {
		return true
	}
	return strings.TrimSpace(r.Header.Get("X-Telemetry-Token")) == token
}

func TelemetryEventFromMetadata(metadata ReviewMetadata) ReviewTelemetryEvent {
	event := ReviewTelemetryEvent{
		SchemaVersion: ReviewTelemetryEventSchemaV1,
		ReviewID:      metadata.ReviewID,
		Status:        metadata.Status,
		Verdict:       metadata.Verdict,
		Profile:       metadata.Profile,
		Model:         metadata.Model,
		BaseRef:       metadata.BaseRef,
		HeadRef:       metadata.HeadRef,
		HeadSHA:       metadata.HeadSHA,
		TokenUsage:    metadata.TokenUsage,
		StartedAt:     metadata.StartedAt,
		FinishedAt:    metadata.FinishedAt,
	}
	if metadata.CostUSD != nil {
		event.CostUSD = *metadata.CostUSD
	}
	return event
}

func containsRawTelemetryField(body []byte) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	for key := range raw {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		switch normalized {
		case "source", "source_code", "raw_source", "diff", "raw_diff", "prompt", "raw_prompt", "log", "logs", "debug_log", "debug_log_path", "report", "raw_report":
			return true
		}
	}
	return false
}

func roundTelemetryUSD(value float64) float64 {
	return math.Round(value*10000) / 10000
}
