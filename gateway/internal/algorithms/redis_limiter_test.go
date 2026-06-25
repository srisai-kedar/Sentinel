package algorithms

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	sredis "github.com/sentinel-project/sentinel/gateway/internal/redis"
)

func TestRedisTokenBucket_AtomicUnderConcurrency(t *testing.T) {
	url := os.Getenv("SENTINEL_REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379"
	}

	client, err := sredis.NewClient(url, false)
	if err != nil {
		t.Skip("redis not available:", err)
	}
	defer client.Close()

	lim := NewRedisTokenBucket(client, 10, 0.01)
	ctx := context.Background()
	key := "test-concurrent-" + time.Now().Format("150405")

	var allowed, denied int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := lim.Allow(ctx, key)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				denied++
				return
			}
			if res.Allowed {
				allowed++
			} else {
				denied++
			}
		}()
	}
	wg.Wait()

	if allowed != 10 {
		t.Fatalf("expected exactly 10 allowed (atomic), got %d allowed / %d denied", allowed, denied)
	}
}
