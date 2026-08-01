package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/markcallen/codex-reviewer/internal/usage"
)

func TestTelemetryEventFromMetadata(t *testing.T) {
	cost := 0.0123
	event := TelemetryEventFromMetadata(ReviewMetadata{
		ReviewID:   "review-1",
		Status:     "succeeded",
		Verdict:    "no_blocking_findings",
		Profile:    "standard",
		Model:      "gpt-5.5",
		BaseRef:    "origin/main",
		HeadRef:    "HEAD",
		HeadSHA:    "abc123",
		TokenUsage: usage.ActualTokenUsage{Status: "available", InputTokens: 100, OutputTokens: 20},
		CostUSD:    &cost,
		StartedAt:  "2026-08-01T00:00:00Z",
		FinishedAt: "2026-08-01T00:01:00Z",
	})
	if event.SchemaVersion != ReviewTelemetryEventSchemaV1 || event.ReviewID != "review-1" || event.CostUSD != cost {
		t.Fatalf("event = %#v", event)
	}
}

func TestTelemetryIngestionQueryAndExport(t *testing.T) {
	server := newTestTelemetryServer(t)
	handler := server.Handler()
	body := `{
		"schema_version":"codex-reviewer.review_telemetry.v1",
		"review_id":"review-1",
		"status":"succeeded",
		"verdict":"approve_with_fixes",
		"profile":"standard",
		"model":"gpt-5.5",
		"token_usage":{"status":"available","input_tokens":100,"output_tokens":20},
		"cost_usd":0.12345
	}`
	rec := telemetryRequest(handler, http.MethodPost, "/telemetry/v1/events", "test-token", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status = %d, body = %s", rec.Code, rec.Body.String())
	}

	spendRec := telemetryRequest(handler, http.MethodGet, "/telemetry/v1/spend", "test-token", "")
	if spendRec.Code != http.StatusOK {
		t.Fatalf("spend status = %d, body = %s", spendRec.Code, spendRec.Body.String())
	}
	var spend SpendRollup
	if err := json.Unmarshal(spendRec.Body.Bytes(), &spend); err != nil {
		t.Fatalf("Unmarshal spend error = %v", err)
	}
	if spend.Events != 1 || spend.AvailableUsage != 1 || spend.TotalCostUSD != 0.1235 || spend.ByModel["gpt-5.5"] != 0.1235 {
		t.Fatalf("spend = %#v", spend)
	}

	effRec := telemetryRequest(handler, http.MethodGet, "/telemetry/v1/effectiveness", "test-token", "")
	if effRec.Code != http.StatusOK {
		t.Fatalf("effectiveness status = %d, body = %s", effRec.Code, effRec.Body.String())
	}
	var effectiveness EffectivenessRollup
	if err := json.Unmarshal(effRec.Body.Bytes(), &effectiveness); err != nil {
		t.Fatalf("Unmarshal effectiveness error = %v", err)
	}
	if effectiveness.ByStatus["succeeded"] != 1 || effectiveness.ByVerdict["approve_with_fixes"] != 1 {
		t.Fatalf("effectiveness = %#v", effectiveness)
	}

	exportRec := telemetryRequest(handler, http.MethodGet, "/telemetry/v1/export", "test-token", "")
	if exportRec.Code != http.StatusOK || !strings.Contains(exportRec.Body.String(), `"review_id":"review-1"`) {
		t.Fatalf("export status = %d, body = %s", exportRec.Code, exportRec.Body.String())
	}
}

func TestTelemetryAuthFailures(t *testing.T) {
	handler := newTestTelemetryServer(t).Handler()
	rec := telemetryRequest(handler, http.MethodGet, "/telemetry/v1/spend", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestTelemetryRejectsOversizedAndRawPayloads(t *testing.T) {
	server, err := NewTelemetryServer(TelemetryOptions{Token: "test-token", MaxBodyBytes: 32})
	if err != nil {
		t.Fatalf("NewTelemetryServer() error = %v", err)
	}
	handler := server.Handler()
	rec := telemetryRequest(handler, http.MethodPost, "/telemetry/v1/events", "test-token", strings.Repeat("x", 64))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, body = %s", rec.Code, rec.Body.String())
	}

	handler = newTestTelemetryServer(t).Handler()
	rec = telemetryRequest(handler, http.MethodPost, "/telemetry/v1/events", "test-token", `{"review_id":"review-1","diff":"secret diff"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("raw payload status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestTelemetrySanitizesObviousSecrets(t *testing.T) {
	handler := newTestTelemetryServer(t).Handler()
	rec := telemetryRequest(handler, http.MethodPost, "/telemetry/v1/events", "test-token", `{"review_id":"api_key=plain-secret","status":"succeeded"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "plain-secret") || !strings.Contains(rec.Body.String(), "[REDACTED]") {
		t.Fatalf("response not redacted: %s", rec.Body.String())
	}
}

func newTestTelemetryServer(t *testing.T) *TelemetryServer {
	t.Helper()
	server, err := NewTelemetryServer(TelemetryOptions{
		Token: "test-token",
		Now:   func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewTelemetryServer() error = %v", err)
	}
	return server
}

func telemetryRequest(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	reader := bytes.NewReader([]byte(body))
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
