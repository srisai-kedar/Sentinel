package adaptive

import (
	"sync"
	"time"

	"github.com/sentinel-project/sentinel/gateway/internal/config"
)

// Controller implements AIMD-style adaptive rate limiting based on observed latency.
//
// Tradeoff: tightening limits under latency spikes protects downstream services but
// may throttle legitimate traffic during transient slowness. We use a floor
// (MinMultiplier) so limits never drop to zero — safe fallback to configured base.
//
// AIMD (Additive Increase, Multiplicative Decrease) — same family as TCP congestion control:
//   - Latency above threshold → multiply limit by 0.5 (fast backoff)
//   - Latency below threshold/2 → add 0.1 to multiplier (slow recovery)
type Controller struct {
	mu sync.Mutex

	enabled          bool
	latencyThreshold time.Duration
	minMultiplier    float64
	multiplier       float64

	// Rolling latency samples for adjustment decisions.
	samples    []time.Duration
	maxSamples int
	lastAdjust time.Time
	adjustEvery time.Duration
}

// New creates an adaptive controller from config.
func New(cfg config.AdaptiveConfig) *Controller {
	if !cfg.Enabled {
		return nil
	}
	return &Controller{
		enabled:          true,
		latencyThreshold: time.Duration(cfg.LatencyThresholdMs) * time.Millisecond,
		minMultiplier:    cfg.MinMultiplier,
		multiplier:       1.0,
		maxSamples:       100,
		adjustEvery:      time.Duration(cfg.AdjustIntervalSec) * time.Second,
		lastAdjust:       time.Now(),
	}
}

// RecordLatency feeds a request duration sample into the controller.
func (c *Controller) RecordLatency(d time.Duration) {
	if c == nil || !c.enabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.samples = append(c.samples, d)
	if len(c.samples) > c.maxSamples {
		c.samples = c.samples[len(c.samples)-c.maxSamples:]
	}

	if time.Since(c.lastAdjust) < c.adjustEvery {
		return
	}
	c.adjust()
	c.lastAdjust = time.Now()
}

func (c *Controller) adjust() {
	if len(c.samples) == 0 {
		return
	}

	var sum time.Duration
	for _, s := range c.samples {
		sum += s
	}
	avg := sum / time.Duration(len(c.samples))

	switch {
	case avg > c.latencyThreshold:
		c.multiplier *= 0.5
		if c.multiplier < c.minMultiplier {
			c.multiplier = c.minMultiplier
		}
	case avg < c.latencyThreshold/2:
		c.multiplier += 0.1
		if c.multiplier > 1.0 {
			c.multiplier = 1.0
		}
	}
	c.samples = c.samples[:0]
}

// Multiplier returns the current limit scale factor (1.0 = no adjustment).
func (c *Controller) Multiplier() float64 {
	if c == nil || !c.enabled {
		return 1.0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.multiplier
}

// AdjustRule scales capacity and refill rate by the current multiplier.
// Window size is unchanged — adaptive tuning applies to throughput, not window span.
func (c *Controller) AdjustRule(rule config.RateLimitRule) config.RateLimitRule {
	m := c.Multiplier()
	if m >= 1.0 {
		return rule
	}
	adjusted := rule
	if adjusted.Capacity > 1 {
		cap := int(float64(adjusted.Capacity) * m)
		if cap < 1 {
			cap = 1
		}
		adjusted.Capacity = cap
	}
	if adjusted.RefillRate > 0 {
		adjusted.RefillRate *= m
		if adjusted.RefillRate < 0.01 {
			adjusted.RefillRate = 0.01
		}
	}
	return adjusted
}
