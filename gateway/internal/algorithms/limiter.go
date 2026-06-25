// Package algorithms implements interchangeable rate-limiting strategies.
//
// Each algorithm is isolated in its own file with documented time/space complexity.
// Phase 1 uses in-memory implementations; Phase 2+ adds Redis-backed variants.
package algorithms

import (
	"context"
	"time"
)

// Result is returned by every Allow call.
type Result struct {
	Allowed    bool
	RetryAfter time.Duration // non-zero when Allowed is false
	Remaining  int
}

// RateLimiter is the strategy interface shared by all algorithms.
// Implementations must be safe for concurrent use.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (Result, error)
}
