package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_OK(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)

	h := HealthHandler()
	h.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", body.Status)
	}
}
