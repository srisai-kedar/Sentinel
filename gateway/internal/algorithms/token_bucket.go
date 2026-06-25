package algorithms

import (
	"context"
	"math"
	"sync"
	"time"
)

// TokenBucket is an in-memory token bucket rate limiter.
//
// Algorithm behavior:
//   - Each client key gets a bucket holding up to `capacity` tokens.
//   - Tokens refill continuously at `refillRate` tokens per second.
//   - Each request consumes one token; if none remain, the request is denied.
//   - Refill is computed lazily on access (not on a background ticker),
//     which avoids goroutine overhead and keeps Allow() self-contained.
//
// Why in-memory for Phase 1:
//   - Proves the allow/deny flow end-to-end before introducing distributed state.
//   - Single-process correctness is easier to unit test in isolation.
//   - Phase 2 moves this same logic into Redis Lua for cross-instance atomicity.
//
// Amortized complexity:
//   - Allow: O(1) amortized — map lookup + constant-time refill math.
//   - Space: O(K) where K = number of distinct client keys seen.
//
// Concurrency: one mutex protects the bucket map. Per-key locking would reduce
// contention but adds complexity; a single mutex is sufficient for Phase 1 MVP.
type TokenBucket struct {
	mu         sync.Mutex
	buckets    map[string]*bucketState
	capacity   float64
	refillRate float64 // tokens per second
}

type bucketState struct {
	tokens     float64
	lastRefill time.Time
}

// NewTokenBucket creates an in-memory token bucket limiter.
// capacity = max burst size; refillRate = sustained tokens/sec.
func NewTokenBucket(capacity int, refillRate float64) *TokenBucket {
	return &TokenBucket{
		buckets:    make(map[string]*bucketState),
		capacity:   float64(capacity),
		refillRate: refillRate,
	}
}

// Allow checks whether `key` has a token available and consumes one if so.
func (tb *TokenBucket) Allow(_ context.Context, key string) (Result, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	state, ok := tb.buckets[key]
	if !ok {
		state = &bucketState{tokens: tb.capacity, lastRefill: now}
		tb.buckets[key] = state
	}

	// Lazy refill: add tokens proportional to elapsed time since last access.
	elapsed := now.Sub(state.lastRefill).Seconds()
	state.tokens = math.Min(tb.capacity, state.tokens+elapsed*tb.refillRate)
	state.lastRefill = now

	if state.tokens >= 1 {
		state.tokens--
		return Result{
			Allowed:   true,
			Remaining: int(math.Floor(state.tokens)),
		}, nil
	}

	// Compute when the next token will be available.
	deficit := 1 - state.tokens
	retryAfter := time.Duration(deficit/tb.refillRate*float64(time.Second)) + time.Millisecond

	return Result{
		Allowed:    false,
		RetryAfter: retryAfter,
		Remaining:  0,
	}, nil
}
