package serverauth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

// parseUsers resolves "login:$PASS_ENV" entries into a credential map.
// Passwords must reference environment variables, mirroring the rule used by
// external sources: no secrets in the config file.
func parseUsers(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("server_auth: basic requires at least one server_users entry (\"login:$PASS_ENV\")")
	}
	users := make(map[string]string, len(entries))
	for _, e := range entries {
		login, ref, ok := strings.Cut(e, ":")
		if !ok || login == "" {
			return nil, fmt.Errorf("server_users: invalid entry %q (want \"login:$PASS_ENV\")", e)
		}
		pass, err := expandSecret("server_users."+login, ref)
		if err != nil {
			return nil, err
		}
		users[login] = pass
	}
	return users, nil
}

// basicAuthMiddleware enforces HTTP Basic auth with constant-time comparison.
func basicAuthMiddleware(next http.Handler, users map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login, pass, ok := r.BasicAuth()
		if !ok || !credentialsMatch(users, login, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="ssg", charset="UTF-8"`)
			http.Error(w, "401 unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// credentialsMatch verifies a login/password pair in constant time.
func credentialsMatch(users map[string]string, login, pass string) bool {
	want, exists := users[login]
	if !exists {
		// Burn comparable time so probing logins is not faster than probing passwords.
		subtle.ConstantTimeCompare([]byte(pass), []byte(pass))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(pass)) == 1
}
