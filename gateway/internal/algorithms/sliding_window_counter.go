package algorithms

import (
	"context"
	"math"
	"sync"
	"time"
)

// SlidingWindowCounter approximates a sliding window using two fixed windows.
//
// At any point in time we blend:
//   weighted = prevCount * (1 - elapsed/window) + currCount
//
// Memory-light (O(1) per key) but approximate — the classic production tradeoff.
//
// Amortized complexity:
//   - Allow: O(1)
//   - Space: O(1) per client key
type SlidingWindowCounter struct {
	mu     sync.Mutex
	state  map[string]*counterState
	limit  int
	window time.Duration
}

type counterState struct {
	currCount   int
	prevCount   int
	windowStart time.Time
}

func NewSlidingWindowCounter(limit int, windowSec int) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		state:  make(map[string]*counterState),
		limit:  limit,
		window: time.Duration(windowSec) * time.Second,
	}
}

func (sw *SlidingWindowCounter) Allow(_ context.Context, key string) (Result, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	st, ok := sw.state[key]
	if !ok {
		st = &counterState{windowStart: now}
		sw.state[key] = st
	}

	if now.Sub(st.windowStart) >= sw.window {
		st.prevCount = st.currCount
		st.currCount = 0
		st.windowStart = now
	}

	elapsed := now.Sub(st.windowStart)
	weight := 1.0 - elapsed.Seconds()/sw.window.Seconds()
	if weight < 0 {
		weight = 0
	}
	estimated := float64(st.prevCount)*weight + float64(st.currCount)

	if int(math.Floor(estimated)) >= sw.limit {
		retryAfter := sw.window - elapsed
		return Result{Allowed: false, RetryAfter: retryAfter, Remaining: 0}, nil
	}

	st.currCount++
	remaining := sw.limit - int(math.Ceil(estimated)) - 1
	if remaining < 0 {
		remaining = 0
	}
	return Result{Allowed: true, Remaining: remaining}, nil
}
