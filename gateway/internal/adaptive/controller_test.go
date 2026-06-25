package adaptive

import (
	"testing"
	"time"

	"github.com/sentinel-project/sentinel/gateway/internal/config"
)

func TestController_DecreasesOnHighLatency(t *testing.T) {
	c := New(config.AdaptiveConfig{
		Enabled:            true,
		LatencyThresholdMs: 50,
		MinMultiplier:      0.25,
		AdjustIntervalSec:  0,
	})
	for i := 0; i < 10; i++ {
		c.RecordLatency(100 * time.Millisecond)
	}
	if c.Multiplier() >= 1.0 {
		t.Fatalf("expected multiplier < 1 after high latency, got %f", c.Multiplier())
	}
}

func TestController_FloorAtMinMultiplier(t *testing.T) {
	c := New(config.AdaptiveConfig{
		Enabled:            true,
		LatencyThresholdMs: 10,
		MinMultiplier:      0.25,
		AdjustIntervalSec:  0,
	})
	for i := 0; i < 50; i++ {
		c.RecordLatency(500 * time.Millisecond)
	}
	if c.Multiplier() < 0.25 {
		t.Fatalf("multiplier %f below floor 0.25", c.Multiplier())
	}
}

func TestController_AdjustRuleScalesCapacity(t *testing.T) {
	c := New(config.AdaptiveConfig{
		Enabled:            true,
		LatencyThresholdMs: 10,
		MinMultiplier:      0.5,
		AdjustIntervalSec:  0,
	})
	for i := 0; i < 5; i++ {
		c.RecordLatency(200 * time.Millisecond)
	}
	rule := config.RateLimitRule{Capacity: 10, RefillRate: 2}
	adj := c.AdjustRule(rule)
	if adj.Capacity >= rule.Capacity {
		t.Fatalf("expected reduced capacity, got %d", adj.Capacity)
	}
}

func TestController_DisabledReturnsNil(t *testing.T) {
	if New(config.AdaptiveConfig{Enabled: false}) != nil {
		t.Fatal("disabled config should return nil controller")
	}
}
