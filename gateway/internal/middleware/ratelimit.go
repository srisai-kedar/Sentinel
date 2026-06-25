package middleware

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/sentinel-project/sentinel/gateway/internal/adaptive"
	"github.com/sentinel-project/sentinel/gateway/internal/algorithms"
	"github.com/sentinel-project/sentinel/gateway/internal/config"
)

// LimiterFactory creates a RateLimiter for a given route rule.
type LimiterFactory func(rule config.RateLimitRule) algorithms.RateLimiter

// RateLimit middleware enforces per-client, per-route quotas.
type RateLimit struct {
	cfg      *config.Config
	factory  LimiterFactory
	adaptive *adaptive.Controller
	limiters sync.Map
}

// NewRateLimit creates rate-limit middleware backed by the given factory.
func NewRateLimit(cfg *config.Config, factory LimiterFactory, adapt *adaptive.Controller) *RateLimit {
	return &RateLimit{cfg: cfg, factory: factory, adaptive: adapt}
}

func (rl *RateLimit) effectiveRule(route string) config.RateLimitRule {
	rule := rl.cfg.RuleForRoute(route)
	if rl.adaptive != nil {
		rule = rl.adaptive.AdjustRule(rule)
	}
	return rule
}

func (rl *RateLimit) limiterForRoute(route string) algorithms.RateLimiter {
	rule := rl.effectiveRule(route)
	mult := 1.0
	if rl.adaptive != nil {
		mult = rl.adaptive.Multiplier()
	}
	key := fmt.Sprintf("%s:%s:%d:%.4f:%d:%.2f", route, rule.Algorithm, rule.Capacity, rule.RefillRate, rule.Window, mult)

	if v, ok := rl.limiters.Load(key); ok {
		return v.(algorithms.RateLimiter)
	}

	lim := rl.factory(rule)
	actual, _ := rl.limiters.LoadOrStore(key, lim)
	return actual.(algorithms.RateLimiter)
}

// InvalidateCache clears cached limiters after runtime config updates.
func (rl *RateLimit) InvalidateCache() {
	rl.limiters = sync.Map{}
}

// Handler returns chi-compatible middleware.
func (rl *RateLimit) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.URL.Path
		clientID := ClientKey(r)
		bucketKey := BuildBucketKey(rl.cfg.Region, route, clientID)

		lim := rl.limiterForRoute(route)
		result, err := lim.Allow(r.Context(), bucketKey)
		if err != nil {
			// Fail-closed: Redis or backend errors deny the request.
			http.Error(w, "rate limiter unavailable", http.StatusServiceUnavailable)
			return
		}

		if !result.Allowed {
			retrySec := int(result.RetryAfter.Seconds()) + 1
			if retrySec < 1 {
				retrySec = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retrySec))
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		next.ServeHTTP(w, r)
	})
}

// ClientKey extracts a stable client identifier from the request.
// Prefers X-Real-IP (set by Nginx) over RemoteAddr.
func ClientKey(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// BuildBucketKey creates a scoped key: region + route + client.
// Region prefix supports multi-region deployments without key collisions.
func BuildBucketKey(region, route, clientID string) string {
	raw := region + ":" + route + ":" + clientID
	if len(raw) <= 128 {
		return raw
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(raw))
	return fmt.Sprintf("%s:%x", route, h.Sum64())
}

// MemoryFactory returns in-memory limiters. Phase 3 adds remaining algorithms here.
func MemoryFactory() LimiterFactory {
	return func(rule config.RateLimitRule) algorithms.RateLimiter {
		switch rule.Algorithm {
		case "sliding_window_log":
			return algorithms.NewSlidingWindowLog(rule.Capacity, rule.Window)
		case "sliding_window_counter":
			return algorithms.NewSlidingWindowCounter(rule.Capacity, rule.Window)
		case "leaky_bucket":
			return algorithms.NewLeakyBucket(rule.Capacity, rule.RefillRate)
		default:
			return algorithms.NewTokenBucket(rule.Capacity, rule.RefillRate)
		}
	}
}
