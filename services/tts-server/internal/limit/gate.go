// Package limit bounds load: Gate caps concurrent syntheses behind a short
// queue, RateLimiter is a per-client token bucket.
package limit

import (
	"context"
	"errors"
	"time"
)

// ErrBusy means every slot is taken and the queue is full or the wait timed out.
var ErrBusy = errors.New("engine busy")

// Gate allows `slots` holders at once and lets at most `queue` more wait,
// each for no longer than `wait`.
type Gate struct {
	slots   chan struct{}
	tickets chan struct{} // holders + waiters
	wait    time.Duration
}

// NewGate builds a Gate.
func NewGate(slots, queue int, wait time.Duration) *Gate {
	return &Gate{
		slots:   make(chan struct{}, slots),
		tickets: make(chan struct{}, slots+queue),
		wait:    wait,
	}
}

// Acquire takes a slot, returning a release function, or ErrBusy (or the
// context error) when it cannot get one in time.
func (g *Gate) Acquire(ctx context.Context) (func(), error) {
	select {
	case g.tickets <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	timer := time.NewTimer(g.wait)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
		return func() { <-g.slots; <-g.tickets }, nil
	case <-timer.C:
		<-g.tickets
		return nil, ErrBusy
	case <-ctx.Done():
		<-g.tickets
		return nil, ctx.Err()
	}
}
