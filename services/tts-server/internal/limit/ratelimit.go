package limit

import (
	"math"
	"sync"
	"time"
)

// sweepEvery is how often idle (full) buckets are dropped.
const sweepEvery = time.Minute

// RateLimiter is a token bucket per client key. A nil *RateLimiter allows
// everything.
type RateLimiter struct {
	mu        sync.Mutex
	perSec    float64
	burst     float64
	buckets   map[string]*bucket
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// NewRateLimiter allows perMinute requests per minute per key, with bursts of
// up to burst. It returns nil (no limiting) when perMinute <= 0.
func NewRateLimiter(perMinute float64, burst int) *RateLimiter {
	if perMinute <= 0 {
		return nil
	}
	return &RateLimiter{
		perSec: perMinute / 60, burst: float64(burst),
		buckets: map[string]*bucket{}, now: time.Now,
	}
}

// Allow spends a token for key. When none is left it returns false and how
// long until the next token arrives.
func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, seen: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.perSec)
	b.seen = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.perSec * float64(time.Second))
	return false, wait
}

// sweep forgets buckets that have refilled completely; they would start full
// anyway, so dropping them changes nothing but memory use.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepEvery {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.seen).Seconds()*l.perSec >= l.burst {
			delete(l.buckets, k)
		}
	}
}
