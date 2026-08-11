package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sentinel-project/sentinel/gateway/internal/config"
)

func newTestAdminHandler() *AdminHandler {
	cfg := &config.Config{
		DefaultLimit: config.RateLimitRule{
			Algorithm:  "token_bucket",
			Capacity:   10,
			RefillRate: 1,
			Window:     60,
		},
		RouteLimits: make(map[string]config.RateLimitRule),
	}

	return &AdminHandler{
		cfg: cfg,
	}
}

func newUpdateLimitRequest(route string, body string) *http.Request {
	req := httptest.NewRequest(
		http.MethodPost,
		"/admin/limits/"+route,
		bytes.NewBufferString(body),
	)

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("route", route)

	return req.WithContext(
		context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx),
	)
}

func TestUpdateLimitRejectsInvalidAlgorithm(t *testing.T) {
	h := newTestAdminHandler()

	req := newUpdateLimitRequest("test", `{
		"algorithm": "invalid",
		"capacity": 10,
		"refill_rate": 1,
		"window_sec": 60
	}`)

	rec := httptest.NewRecorder()

	h.UpdateLimit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	if _, ok := h.cfg.RouteLimits["test"]; ok {
		t.Fatal("invalid configuration should not be applied")
	}
}

func TestUpdateLimitRejectsInvalidCapacity(t *testing.T) {
	h := newTestAdminHandler()

	req := newUpdateLimitRequest("test", `{
		"algorithm": "token_bucket",
		"capacity": 0,
		"refill_rate": 1,
		"window_sec": 60
	}`)

	rec := httptest.NewRecorder()

	h.UpdateLimit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	if _, ok := h.cfg.RouteLimits["test"]; ok {
		t.Fatal("invalid configuration should not be applied")
	}
}

func TestUpdateLimitRejectsNegativeRefillRate(t *testing.T) {
	h := newTestAdminHandler()

	req := newUpdateLimitRequest("test", `{
		"algorithm": "token_bucket",
		"capacity": 10,
		"refill_rate": -1,
		"window_sec": 60
	}`)

	rec := httptest.NewRecorder()

	h.UpdateLimit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	if _, ok := h.cfg.RouteLimits["test"]; ok {
		t.Fatal("invalid configuration should not be applied")
	}
}

func TestUpdateLimitRejectsInvalidWindow(t *testing.T) {
	h := newTestAdminHandler()

	req := newUpdateLimitRequest("test", `{
		"algorithm": "token_bucket",
		"capacity": 10,
		"refill_rate": 1,
		"window_sec": -1
	}`)

	rec := httptest.NewRecorder()

	h.UpdateLimit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	if _, ok := h.cfg.RouteLimits["test"]; ok {
		t.Fatal("invalid configuration should not be applied")
	}
}

func TestUpdateLimitAcceptsValidConfiguration(t *testing.T) {
	h := newTestAdminHandler()

	req := newUpdateLimitRequest("test", `{
		"algorithm": "token_bucket",
		"capacity": 20,
		"refill_rate": 2,
		"window_sec": 60
	}`)

	rec := httptest.NewRecorder()

	h.UpdateLimit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	rule, ok := h.cfg.RouteLimits["test"]
	if !ok {
		t.Fatal("valid configuration was not applied")
	}

	if rule.Capacity != 20 {
		t.Fatalf("got capacity %d, want 20", rule.Capacity)
	}

	if rule.RefillRate != 2 {
		t.Fatalf("got refill rate %f, want 2", rule.RefillRate)
	}

	if rule.Window != 60 {
		t.Fatalf("got window %d, want 60", rule.Window)
	}
}
