package algorithms

import (
	"context"
	"sync"
	"time"
)

// SlidingWindowLog tracks exact request timestamps within a sliding window.
//
// Tradeoff vs SlidingWindowCounter:
//   - Log: precise, O(k) memory per key where k = requests in window.
//   - Counter: approximate, O(1) memory — see sliding_window_counter.go.
//
// Data structure: slice of timestamps acting as a deque (two-pointer eviction).
// Expired entries are removed from the front on each Allow() — amortized O(1)
// because each timestamp is inserted once and removed once.
//
// Amortized complexity:
//   - Allow: O(1) amortized (each entry evicted at most once)
//   - Space: O(k) per client key
type SlidingWindowLog struct {
	mu      sync.Mutex
	windows map[string][]time.Time
	limit   int
	window  time.Duration
}

func NewSlidingWindowLog(limit int, windowSec int) *SlidingWindowLog {
	return &SlidingWindowLog{
		windows: make(map[string][]time.Time),
		limit:   limit,
		window:  time.Duration(windowSec) * time.Second,
	}
}

func (sw *SlidingWindowLog) Allow(_ context.Context, key string) (Result, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	log := sw.windows[key]
	i := 0
	for i < len(log) && log[i].Before(cutoff) {
		i++
	}
	log = log[i:]

	if len(log) >= sw.limit {
		oldest := log[0]
		retryAfter := sw.window - now.Sub(oldest)
		if retryAfter < time.Millisecond {
			retryAfter = time.Millisecond
		}
		sw.windows[key] = log
		return Result{Allowed: false, RetryAfter: retryAfter, Remaining: 0}, nil
	}

	log = append(log, now)
	sw.windows[key] = log
	return Result{Allowed: true, Remaining: sw.limit - len(log)}, nil
}
