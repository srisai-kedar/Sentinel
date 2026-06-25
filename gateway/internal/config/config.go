package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds gateway settings loaded from environment variables.
type Config struct {
	Port              string
	InstanceID        string
	Region            string // multi-region key prefix (Phase 8 readiness)
	RedisURL          string
	RedisFailureMode  string // "closed" | "open"
	DefaultAlgorithm  string
	AdminAPIKey       string
	DownstreamURL     string
	UseRedis          bool
	Adaptive          AdaptiveConfig

	// Default rate limit applied when no per-route override exists.
	DefaultLimit RateLimitRule

	// Per-route overrides keyed by route pattern (e.g. "/api/test").
	RouteLimits map[string]RateLimitRule
}

// AdaptiveConfig controls AIMD-style limit tightening based on latency.
type AdaptiveConfig struct {
	Enabled            bool
	LatencyThresholdMs int
	MinMultiplier      float64
	AdjustIntervalSec  int
}

// RateLimitRule describes limits for a single route.
type RateLimitRule struct {
	Algorithm  string  // token_bucket | sliding_window_log | sliding_window_counter | leaky_bucket
	Capacity   int     // max burst / window size
	RefillRate float64 // tokens per second (token bucket / leaky bucket)
	Window     int     // window size in seconds (sliding window algorithms)
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:             envOr("SENTINEL_PORT", "8080"),
		InstanceID:       envOr("SENTINEL_INSTANCE_ID", "gateway-1"),
		Region:           envOr("SENTINEL_REGION", "local"),
		RedisURL:         envOr("SENTINEL_REDIS_URL", "redis://localhost:6379"),
		RedisFailureMode: envOr("SENTINEL_REDIS_FAILURE_MODE", "closed"),
		DefaultAlgorithm: envOr("SENTINEL_ALGORITHM", "token_bucket"),
		AdminAPIKey:      envOr("SENTINEL_ADMIN_API_KEY", "dev-admin-key"),
		DownstreamURL:    envOr("SENTINEL_DOWNSTREAM_URL", ""),
		UseRedis:         envOr("SENTINEL_USE_REDIS", "false") == "true",
		Adaptive: AdaptiveConfig{
			Enabled:            envOr("SENTINEL_ADAPTIVE_ENABLED", "false") == "true",
			LatencyThresholdMs: envInt("SENTINEL_ADAPTIVE_LATENCY_MS", 100),
			MinMultiplier:      envFloat("SENTINEL_ADAPTIVE_MIN_MULTIPLIER", 0.25),
			AdjustIntervalSec:  envInt("SENTINEL_ADAPTIVE_ADJUST_SEC", 5),
		},
		DefaultLimit: RateLimitRule{
			Algorithm:  envOr("SENTINEL_ALGORITHM", "token_bucket"),
			Capacity:   envInt("SENTINEL_RATE_LIMIT", 10),
			RefillRate: envFloat("SENTINEL_REFILL_RATE", 1),
			Window:     envInt("SENTINEL_WINDOW_SEC", 60),
		},
		RouteLimits: make(map[string]RateLimitRule),
	}

	// Per-route limits: SENTINEL_ROUTE_LIMITS="/api/test:10:1,/api/heavy:5:0.5"
	if raw := os.Getenv("SENTINEL_ROUTE_LIMITS"); raw != "" {
		for _, entry := range strings.Split(raw, ",") {
			parts := strings.Split(strings.TrimSpace(entry), ":")
			if len(parts) < 3 {
				continue
			}
			capacity, _ := strconv.Atoi(parts[1])
			rate, _ := strconv.ParseFloat(parts[2], 64)
			cfg.RouteLimits[parts[0]] = RateLimitRule{
				Algorithm:  cfg.DefaultAlgorithm,
				Capacity:   capacity,
				RefillRate: rate,
				Window:     cfg.DefaultLimit.Window,
			}
		}
	}

	if cfg.RedisFailureMode != "closed" && cfg.RedisFailureMode != "open" {
		return nil, fmt.Errorf("invalid SENTINEL_REDIS_FAILURE_MODE: %q", cfg.RedisFailureMode)
	}

	return cfg, nil
}

// RuleForRoute returns the rate limit rule for a given route path.
func (c *Config) RuleForRoute(route string) RateLimitRule {
	if rule, ok := c.RouteLimits[route]; ok {
		return rule
	}
	return c.DefaultLimit
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
