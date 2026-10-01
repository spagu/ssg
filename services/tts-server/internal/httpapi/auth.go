package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/limit"
)

// keySet holds SHA-256 digests of bearer keys, so comparisons are fixed-length
// and constant-time and the raw keys are not kept around.
type keySet [][32]byte

func newKeySet(keys []string) keySet {
	out := make(keySet, 0, len(keys))
	for _, k := range keys {
		out = append(out, sha256.Sum256([]byte(k)))
	}
	return out
}

// match compares against every key without early exit and returns the
// matching digest.
func (s keySet) match(key string) ([32]byte, bool) {
	sum := sha256.Sum256([]byte(key))
	var found [32]byte
	ok := 0
	for _, k := range s {
		eq := subtle.ConstantTimeCompare(sum[:], k[:])
		subtle.ConstantTimeCopy(eq, found[:], k[:])
		ok |= eq
	}
	return found, ok == 1
}

type ctxKey struct{}

// clientID is the rate-limit identity: a key-digest prefix when
// authenticated (never the key itself), else the remote IP.
func clientID(r *http.Request) string {
	if id, ok := r.Context().Value(ctxKey{}).(string); ok {
		return id
	}
	return "ip:" + remoteIP(r)
}

// remoteIP is the peer address without the port. X-Forwarded-For is not
// trusted: put the server behind a proxy that rate-limits on its own if needed.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// bearer extracts the token from "Authorization: Bearer <token>".
func bearer(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// requireKey admits requests carrying one of keys. With open set (no keys
// configured for this level) every request passes.
func requireKey(keys keySet, open bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if open {
			next.ServeHTTP(w, r)
			return
		}
		digest, ok := keys.match(bearer(r))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tts"`)
			writeError(w, http.StatusUnauthorized, codeUnauthorized, "missing or invalid bearer key")
			return
		}
		id := "key:" + hex.EncodeToString(digest[:6])
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// requireAdmin refuses everything when no admin keys exist.
func requireAdmin(keys keySet, next http.Handler) http.Handler {
	if len(keys) == 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusForbidden, codeForbidden, "voice management is disabled (set TTS_ADMIN_KEYS)")
		})
	}
	return requireKey(keys, false, next)
}

// rateLimit spends one token per request for the client identity.
func rateLimit(l *limit.RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := l.Allow(clientID(r)); !ok {
			w.Header().Set("Retry-After", retryAfter(wait))
			writeError(w, http.StatusTooManyRequests, codeRateLimited, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// retryAfter renders a wait as whole seconds, at least 1.
func retryAfter(d time.Duration) string {
	return strconv.Itoa(max(1, int((d+time.Second-1)/time.Second)))
}
