package serverauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// HS256 JWT verification with the standard library only. Restricting the
// implementation to a single algorithm removes the classic alg-confusion
// attacks by construction; RS256/JWKS (and SSO/LDAP) are deferred.

// jwtMiddleware accepts requests carrying a valid `Authorization: Bearer`
// token signed with the shared HS256 secret.
func jwtMiddleware(next http.Handler, secret []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !verifyHS256(strings.TrimSpace(token), secret, time.Now()) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ssg"`)
			http.Error(w, "401 unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// verifyHS256 checks the token structure, algorithm, signature and the
// exp/nbf claims (when present) against now.
func verifyHS256(token string, secret []byte, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if !decodeSegment(parts[0], &header) || header.Alg != "HS256" {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return false
	}
	var claims struct {
		Exp int64 `json:"exp"`
		Nbf int64 `json:"nbf"`
	}
	if !decodeSegment(parts[1], &claims) {
		return false
	}
	if claims.Exp != 0 && now.Unix() >= claims.Exp {
		return false
	}
	if claims.Nbf != 0 && now.Unix() < claims.Nbf {
		return false
	}
	return true
}

// decodeSegment base64url-decodes and JSON-parses one token segment.
func decodeSegment(segment string, into interface{}) bool {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, into) == nil
}
