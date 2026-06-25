package circuit_breaker

import (
	"net/http"
	"sync"
	"time"
)

// State represents the circuit breaker state machine.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Breaker protects a downstream service from cascading failures.
//
// State machine:
//   Closed ──(failures >= threshold)──► Open
//   Open ──(timeout elapsed)──► HalfOpen
//   HalfOpen ──(probe succeeds)──► Closed
//   HalfOpen ──(probe fails)──► Open
type Breaker struct {
	mu sync.Mutex

	name             string
	state            State
	failures         int
	failureThreshold int
	openTimeout      time.Duration
	openedAt         time.Time
	halfOpenInFlight bool
}

// New creates a circuit breaker with the given failure threshold and open timeout.
func New(name string, failureThreshold int, openTimeout time.Duration) *Breaker {
	return &Breaker{
		name:             name,
		state:            Closed,
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
	}
}

// Name returns the breaker identifier (typically downstream host).
func (b *Breaker) Name() string { return b.name }

// State returns the current breaker state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maybeTransitionToHalfOpen()
	return b.state
}

// Allow reports whether a request may proceed to the downstream service.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.maybeTransitionToHalfOpen()

	switch b.state {
	case Closed:
		return true
	case Open:
		return false
	case HalfOpen:
		if b.halfOpenInFlight {
			return false
		}
		b.halfOpenInFlight = true
		return true
	default:
		return false
	}
}

// RecordSuccess marks a downstream call as successful.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures = 0
	b.halfOpenInFlight = false
	b.state = Closed
}

// RecordFailure marks a downstream call as failed and may trip the breaker.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.halfOpenInFlight = false
	b.failures++

	if b.state == HalfOpen || b.failures >= b.failureThreshold {
		b.state = Open
		b.openedAt = time.Now()
		b.failures = 0
	}
}

func (b *Breaker) maybeTransitionToHalfOpen() {
	if b.state == Open && time.Since(b.openedAt) >= b.openTimeout {
		b.state = HalfOpen
		b.halfOpenInFlight = false
	}
}

// Middleware wraps downstream proxy calls with circuit breaker protection.
type Middleware struct {
	breakers sync.Map // name -> *Breaker
}

func NewMiddleware() *Middleware {
	return &Middleware{}
}

func (m *Middleware) Get(name string) *Breaker {
	if v, ok := m.breakers.Load(name); ok {
		return v.(*Breaker)
	}
	b := New(name, 5, 10*time.Second)
	actual, _ := m.breakers.LoadOrStore(name, b)
	return actual.(*Breaker)
}

// Handler returns middleware that fast-fails with 503 when the breaker is open.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		br := m.Get(name)

		if !br.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"circuit breaker open"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
