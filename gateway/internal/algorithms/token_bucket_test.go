package algorithms

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucket_AllowsBurstThenDenies(t *testing.T) {
	tb := NewTokenBucket(3, 1) // 3 token burst, 1/sec refill
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		res, err := tb.Allow(ctx, "client-a")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	res, err := tb.Allow(ctx, "client-a")
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Fatal("4th request should be denied")
	}
	if res.RetryAfter <= 0 {
		t.Fatal("expected positive Retry-After")
	}
}

func TestTokenBucket_RefillsOverTime(t *testing.T) {
	tb := NewTokenBucket(1, 10) // fast refill for test
	ctx := context.Background()

	if _, err := tb.Allow(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	if res, _ := tb.Allow(ctx, "c"); res.Allowed {
		t.Fatal("should be denied immediately after consuming token")
	}

	time.Sleep(150 * time.Millisecond)
	res, err := tb.Allow(ctx, "c")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed {
		t.Fatal("token should have refilled")
	}
}

func TestTokenBucket_IsolatedPerKey(t *testing.T) {
	tb := NewTokenBucket(1, 1)
	ctx := context.Background()

	if res, _ := tb.Allow(ctx, "a"); !res.Allowed {
		t.Fatal("a should be allowed")
	}
	if res, _ := tb.Allow(ctx, "b"); !res.Allowed {
		t.Fatal("b should have its own bucket")
	}
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	tb := NewTokenBucket(100, 100)
	ctx := context.Background()
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			_, _ = tb.Allow(ctx, "concurrent")
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	// No race detector failures = pass; some requests may be denied.
}
