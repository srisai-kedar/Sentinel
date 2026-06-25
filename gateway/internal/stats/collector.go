package stats

import (
	"sort"
	"sync"
	"time"
)

// Snapshot is a point-in-time view of gateway traffic.
type Snapshot struct {
	Timestamp      time.Time      `json:"timestamp"`
	RequestsPerSec float64        `json:"requests_per_sec"`
	Allowed        int64          `json:"allowed"`
	Blocked        int64          `json:"blocked"`
	TopOffenders   []Offender     `json:"top_offenders"`
	InstanceID     string         `json:"instance_id"`
}

// Offender tracks a high-volume client.
type Offender struct {
	Client  string `json:"client"`
	Route   string `json:"route"`
	Blocked int64  `json:"blocked"`
	Total   int64  `json:"total"`
}

// Collector aggregates live traffic stats for the admin API and WebSocket feed.
type Collector struct {
	mu         sync.Mutex
	instanceID string

	windowStart time.Time
	allowed     int64
	blocked     int64

	clientTotals  map[string]int64
	clientBlocked map[string]int64
}

func New(instanceID string) *Collector {
	return &Collector{
		instanceID:    instanceID,
		windowStart:   time.Now(),
		clientTotals:  make(map[string]int64),
		clientBlocked: make(map[string]int64),
	}
}

func (c *Collector) RecordAllowed(route, client string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allowed++
	key := route + "|" + client
	c.clientTotals[key]++
}

func (c *Collector) RecordBlocked(route, client string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocked++
	key := route + "|" + client
	c.clientTotals[key]++
	c.clientBlocked[key]++
}

func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.windowStart).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}

	offenders := make([]Offender, 0)
	for key, total := range c.clientTotals {
		route, client := splitKey(key)
		offenders = append(offenders, Offender{
			Client:  client,
			Route:   route,
			Blocked: c.clientBlocked[key],
			Total:   total,
		})
	}
	sort.Slice(offenders, func(i, j int) bool {
		if offenders[i].Blocked == offenders[j].Blocked {
			return offenders[i].Total > offenders[j].Total
		}
		return offenders[i].Blocked > offenders[j].Blocked
	})
	if len(offenders) > 10 {
		offenders = offenders[:10]
	}

	return Snapshot{
		Timestamp:      time.Now(),
		RequestsPerSec: float64(c.allowed+c.blocked) / elapsed,
		Allowed:        c.allowed,
		Blocked:        c.blocked,
		TopOffenders:   offenders,
		InstanceID:     c.instanceID,
	}
}

func splitKey(key string) (route, client string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}
