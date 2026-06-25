package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sentinelcfg "github.com/sentinel-project/sentinel/gateway/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	cfg, err := sentinelcfg.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UseRedis = false

	app, err := newApp(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	app.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestRateLimitReturns429(t *testing.T) {
	cfg, err := sentinelcfg.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UseRedis = false
	cfg.DefaultLimit.Capacity = 1
	cfg.DefaultLimit.RefillRate = 0.001

	app, err := newApp(cfg)
	if err != nil {
		t.Fatal(err)
	}

	handler := app.rateLimitWithStats(http.HandlerFunc(app.handleTest))

	req1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first: got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second: got %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}
