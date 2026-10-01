package limit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGate(t *testing.T) {
	g := NewGate(1, 1, 30*time.Millisecond)
	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// queue slot available, but wait times out
	if _, err := g.Acquire(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	// fill the queue with a waiter, then a third caller is rejected at once
	done := make(chan error)
	go func() {
		r, err := g.Acquire(context.Background())
		if err == nil {
			r()
		}
		done <- err
	}()
	time.Sleep(5 * time.Millisecond)
	if _, err := g.Acquire(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatal("waiter should get the slot:", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	release, _ = g.Acquire(context.Background())
	cancel()
	if _, err := g.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	release()
}

func TestRateLimiter(t *testing.T) {
	if NewRateLimiter(0, 1) != nil {
		t.Fatal("disabled limiter must be nil")
	}
	var none *RateLimiter
	if ok, _ := none.Allow("x"); !ok {
		t.Fatal("nil limiter allows")
	}
	now := time.Unix(1000, 0)
	l := NewRateLimiter(60, 2)
	l.now = func() time.Time { return now }
	for i := range 2 {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatal("burst", i)
		}
	}
	ok, wait := l.Allow("k")
	if ok || wait != time.Second {
		t.Fatal(ok, wait)
	}
	if ok, _ := l.Allow("other"); !ok {
		t.Fatal("keys are independent")
	}
	now = now.Add(2 * time.Minute) // refilled: sweep drops both buckets
	if ok, _ := l.Allow("k"); !ok || len(l.buckets) != 1 {
		t.Fatal("refill/sweep", len(l.buckets))
	}
}
