package algorithms

import (
	"context"
	"math"
	"sync"
	"time"
)

// LeakyBucket enforces a constant outflow rate using queue-based logic.
//
// Requests enter a queue (bucket). The bucket leaks at a fixed rate.
// If the queue is full, the request is rejected.
//
// Amortized complexity:
//   - Allow: O(1) amortized (lazy leak computation on access)
//   - Space: O(1) per client key
type LeakyBucket struct {
	mu       sync.Mutex
	buckets  map[string]*leakyState
	capacity float64
	leakRate float64
}

type leakyState struct {
	volume   float64
	lastLeak time.Time
}

func NewLeakyBucket(capacity int, leakRate float64) *LeakyBucket {
	return &LeakyBucket{
		buckets:  make(map[string]*leakyState),
		capacity: float64(capacity),
		leakRate: leakRate,
	}
}

func (lb *LeakyBucket) Allow(_ context.Context, key string) (Result, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	now := time.Now()
	st, ok := lb.buckets[key]
	if !ok {
		st = &leakyState{lastLeak: now}
		lb.buckets[key] = st
	}

	elapsed := now.Sub(st.lastLeak).Seconds()
	st.volume = math.Max(0, st.volume-elapsed*lb.leakRate)
	st.lastLeak = now

	if st.volume+1 <= lb.capacity {
		st.volume++
		return Result{
			Allowed:   true,
			Remaining: int(lb.capacity - st.volume),
		}, nil
	}

	overflow := st.volume + 1 - lb.capacity
	retryAfter := time.Duration(overflow/lb.leakRate*float64(time.Second)) + time.Millisecond
	return Result{Allowed: false, RetryAfter: retryAfter, Remaining: 0}, nil
}
