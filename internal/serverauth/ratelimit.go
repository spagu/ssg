package serverauth

import (
	"net/http"
	"sync"
	"time"
)

// A small token-bucket limiter per client IP (standard library only). Buckets
// refill continuously at rate tokens/second up to burst; an empty bucket
// yields 429 with Retry-After.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
	now     func() time.Time // injectable clock for tests
}

// bucket tracks one client's remaining tokens.
type bucket struct {
	tokens float64
	last   time.Time
}

// maxBuckets bounds the per-IP map; beyond it, stale entries are purged.
const maxBuckets = 4096

// newLimiter builds a limiter; burst defaults to 2×rate (minimum 1).
func newLimiter(rate float64, burst int) *limiter {
	b := float64(burst)
	if b <= 0 {
		b = rate * 2
	}
	if b < 1 {
		b = 1
	}
	return &limiter{buckets: map[string]*bucket{}, rate: rate, burst: b, now: time.Now}
}

// allow consumes one token for the key, reporting whether the request may pass.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	bk := l.buckets[key]
	if bk == nil {
		if len(l.buckets) >= maxBuckets {
			l.purge(now)
		}
		bk = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = bk
	}
	bk.tokens += now.Sub(bk.last).Seconds() * l.rate
	if bk.tokens > l.burst {
		bk.tokens = l.burst
	}
	bk.last = now
	if bk.tokens < 1 {
		return false
	}
	bk.tokens--
	return true
}

// purge drops buckets idle long enough to have refilled completely.
func (l *limiter) purge(now time.Time) {
	idle := time.Duration(l.burst/l.rate*float64(time.Second)) + time.Minute
	for key, bk := range l.buckets {
		if now.Sub(bk.last) > idle {
			delete(l.buckets, key)
		}
	}
}

// rateLimitMiddleware enforces the per-IP limit.
func rateLimitMiddleware(next http.Handler, l *limiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		key := ""
		if ip != nil {
			key = ip.String()
		}
		if !l.allow(key) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
