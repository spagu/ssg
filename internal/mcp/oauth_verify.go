package mcp

// Access-token checks for the OAuth resource server (#362).

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// tokenLeeway absorbs clock skew between this host and the issuer.
const tokenLeeway = time.Minute

// signingAlgorithms are the asymmetric algorithms a token may be signed with.
// HS256 is not among them: a shared secret would let anyone who can verify a
// token also mint one.
var signingAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.PS384, jose.PS512,
	jose.ES256, jose.ES384, jose.ES512, jose.EdDSA,
}

// errInsufficientScope is a valid token without a required scope, which is a
// 403 rather than a 401: signing in again would not help.
var errInsufficientScope = errors.New("insufficient scope")

// tokenVerifier checks access tokens against one issuer and resource.
type tokenVerifier struct {
	opts OAuthOptions
	keys *jwksCache
	now  func() time.Time
}

func newTokenVerifier(o OAuthOptions) *tokenVerifier {
	return &tokenVerifier{opts: o, keys: newJWKSCache(o), now: time.Now}
}

// scopeClaims are the two spellings of granted scopes: `scope`, a
// space-separated string (RFC 9068), and `scp`, which some issuers send as a
// list and others as a string.
type scopeClaims struct {
	Scope string          `json:"scope"`
	Scp   json.RawMessage `json:"scp"`
}

func (s scopeClaims) granted() map[string]bool {
	out := map[string]bool{}
	for _, v := range strings.Fields(s.Scope) {
		out[v] = true
	}
	var list []string
	if json.Unmarshal(s.Scp, &list) != nil {
		var one string
		if json.Unmarshal(s.Scp, &one) == nil {
			list = strings.Fields(one)
		}
	}
	for _, v := range list {
		out[v] = true
	}
	return out
}

// verify returns nil for a token this server accepts.
func (v *tokenVerifier) verify(raw string) error {
	tok, err := jwt.ParseSigned(raw, signingAlgorithms)
	if err != nil {
		return fmt.Errorf("not a signed JWT: %w", err)
	}
	if len(tok.Headers) != 1 {
		return errors.New("exactly one signature expected")
	}
	key, err := v.keys.key(tok.Headers[0].KeyID)
	if err != nil {
		return err
	}
	var claims jwt.Claims
	var scopes scopeClaims
	if err := tok.Claims(key.Key, &claims, &scopes); err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	// go-jose skips a check whose claim is absent; a token without an expiry
	// or an audience is one this server never accepts.
	if claims.Expiry == nil || len(claims.Audience) == 0 {
		return errors.New("token has no exp or aud")
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:      v.opts.Issuer,
		AnyAudience: jwt.Audience{v.opts.Resource},
		Time:        v.now(),
	}, tokenLeeway); err != nil {
		return err
	}
	granted := scopes.granted()
	for _, want := range v.opts.Scopes {
		if !granted[want] {
			return errInsufficientScope
		}
	}
	return nil
}
