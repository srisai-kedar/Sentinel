package circuit_breaker

import (
	"testing"
	"time"
)

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	b := New("downstream", 3, time.Minute)

	for i := 0; i < 3; i++ {
		if !b.Allow() {
			t.Fatal("should allow while closed")
		}
		b.RecordFailure()
	}

	if b.State() != Open {
		t.Fatalf("expected open, got %s", b.State())
	}
	if b.Allow() {
		t.Fatal("should not allow when open")
	}
}

func TestBreaker_HalfOpenRecovery(t *testing.T) {
	b := New("downstream", 1, 20*time.Millisecond)
	b.RecordFailure()
	if b.State() != Open {
		t.Fatal("expected open")
	}

	time.Sleep(25 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe in half-open")
	}
	b.RecordSuccess()
	if b.State() != Closed {
		t.Fatalf("expected closed after success, got %s", b.State())
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := New("downstream", 1, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(15 * time.Millisecond)

	if !b.Allow() {
		t.Fatal("probe should be allowed")
	}
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("expected open after probe failure, got %s", b.State())
	}
}
