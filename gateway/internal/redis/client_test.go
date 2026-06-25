package redis

import (
	"context"
	"testing"
)

func TestRedisClient_FailClosedOnBadHost(t *testing.T) {
	// NewClient pings on connect — invalid host should fail fast.
	_, err := NewClient("redis://127.0.0.1:59999", false)
	if err == nil {
		t.Fatal("expected connection error for invalid redis host")
	}
	_ = context.Background()
}
