package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sentinel-project/sentinel/gateway/internal/algorithms"
	"github.com/sentinel-project/sentinel/gateway/internal/config"
)

type errLimiter struct{}

func (errLimiter) Allow(context.Context, string) (algorithms.Result, error) {
	return algorithms.Result{}, errors.New("redis unavailable")
}

func TestRateLimitMiddleware_FailClosedOnRedisError(t *testing.T) {
	cfg := &config.Config{
		Region: "local",
		DefaultLimit: config.RateLimitRule{Algorithm: "token_bucket", Capacity: 10, RefillRate: 1},
	}
	factory := func(config.RateLimitRule) algorithms.RateLimiter { return errLimiter{} }
	rl := NewRateLimit(cfg, factory, nil)

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("fail-closed: got %d, want 503", rec.Code)
	}
}

func TestRateLimitMiddleware_Returns429WithRetryAfter(t *testing.T) {
	cfg := &config.Config{
		Region: "local",
		DefaultLimit: config.RateLimitRule{
			Algorithm:  "token_bucket",
			Capacity:   1,
			RefillRate: 0.01, // very slow refill
		},
		RouteLimits: map[string]config.RateLimitRule{},
	}

	factory := func(rule config.RateLimitRule) algorithms.RateLimiter {
		return algorithms.NewTokenBucket(rule.Capacity, rule.RefillRate)
	}
	rl := NewRateLimit(cfg, factory, nil)

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request allowed.
	req1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", rec1.Code)
	}

	// Second request denied.
	req2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req2.RemoteAddr = "192.168.1.1:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestClientKey_PrefersXRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "127.0.0.1:9999"
	if got := ClientKey(req); got != "10.0.0.5" {
		t.Fatalf("got %q, want 10.0.0.5", got)
	}
}

func TestBuildBucketKey_HashesLongKeys(t *testing.T) {
	longClient := string(make([]byte, 200))
	key := BuildBucketKey("local", "/api/test", longClient)
	if len(key) > 150 {
		t.Fatalf("expected hashed key, got len %d", len(key))
	}
}
