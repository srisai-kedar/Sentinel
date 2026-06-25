package algorithms

import (
	"context"
	"testing"
	"time"
)

func TestSlidingWindowLog_EnforcesLimit(t *testing.T) {
	sw := NewSlidingWindowLog(2, 60)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		res, err := sw.Allow(ctx, "k")
		if err != nil || !res.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	res, _ := sw.Allow(ctx, "k")
	if res.Allowed {
		t.Fatal("third request should be denied")
	}
}

func TestSlidingWindowCounter_EnforcesLimit(t *testing.T) {
	sw := NewSlidingWindowCounter(2, 60)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		res, _ := sw.Allow(ctx, "k")
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	res, _ := sw.Allow(ctx, "k")
	if res.Allowed {
		t.Fatal("should deny over limit")
	}
}

func TestLeakyBucket_EnforcesCapacity(t *testing.T) {
	lb := NewLeakyBucket(2, 0.01)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		res, _ := lb.Allow(ctx, "k")
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	res, _ := lb.Allow(ctx, "k")
	if res.Allowed {
		t.Fatal("bucket should be full")
	}
}

func TestSlidingWindowLog_WindowExpires(t *testing.T) {
	sw := NewSlidingWindowLog(1, 1) // 1 second window
	ctx := context.Background()

	if res, _ := sw.Allow(ctx, "k"); !res.Allowed {
		t.Fatal("first should pass")
	}
	if res, _ := sw.Allow(ctx, "k"); res.Allowed {
		t.Fatal("second should fail")
	}
	time.Sleep(1100 * time.Millisecond)
	if res, _ := sw.Allow(ctx, "k"); !res.Allowed {
		t.Fatal("should allow after window expires")
	}
}
