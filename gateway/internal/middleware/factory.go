package middleware

import (
	"github.com/sentinel-project/sentinel/gateway/internal/algorithms"
	"github.com/sentinel-project/sentinel/gateway/internal/config"
	sredis "github.com/sentinel-project/sentinel/gateway/internal/redis"
)

// NewFactory returns a LimiterFactory that uses Redis or in-memory backends.
func NewFactory(cfg *config.Config, redisClient *sredis.Client) LimiterFactory {
	return func(rule config.RateLimitRule) algorithms.RateLimiter {
		if cfg.UseRedis && redisClient != nil {
			switch rule.Algorithm {
			case "sliding_window_log":
				return algorithms.NewRedisSlidingWindowLog(redisClient, rule.Capacity, rule.Window)
			case "sliding_window_counter":
				return algorithms.NewRedisSlidingWindowCounter(redisClient, rule.Capacity, rule.Window)
			case "leaky_bucket":
				return algorithms.NewRedisLeakyBucket(redisClient, rule.Capacity, rule.RefillRate)
			default:
				return algorithms.NewRedisTokenBucket(redisClient, rule.Capacity, rule.RefillRate)
			}
		}

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
