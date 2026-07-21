package externalsource

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Secrets come exclusively from environment variables (plan §Jeden system
// sekretów): a value of "$NAME" resolves to os.Getenv("NAME"). Auth secret
// fields REQUIRE the env form so credentials never live in the config file,
// and error messages only ever name the variable, never its value.

// UnsetEnvError reports a config value that references an environment variable
// which is not set (or is empty). It is a distinct type because an *optional*
// source hitting it is skipped with a warning rather than failing the build —
// one config can then carry an env-driven source the whole team need not set
// (issue #35). The value is never included, only the variable name.
type UnsetEnvError struct {
	Source string
	Field  string
	Name   string
}

// Error formats the message naming the variable that must be exported.
func (e *UnsetEnvError) Error() string {
	return fmt.Sprintf("external source %q: %s references $%s, which is not set in the environment", e.Source, e.Field, e.Name)
}

// envRefRe matches the two reference spellings plus the "$$" escape:
// "$NAME", "${NAME}" and "$$" (a literal dollar). Anything else containing a
// "$" — "$5", "a$" — is left untouched, so prices and jQuery-ish strings in
// headers survive expansion (GO-055).
var envRefRe = regexp.MustCompile(`\$(?:\$|\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)`)

// expandEnvInline expands every "$NAME"/"${NAME}" reference inside value, so a
// URL like "$API_BASE/api/products" works and one config can switch between
// environments. "$$" collapses to a literal "$". The first unset variable wins
// and is reported as an *UnsetEnvError.
func expandEnvInline(source, field, value string) (string, error) {
	if !strings.Contains(value, "$") {
		return value, nil
	}
	var firstErr error
	out := envRefRe.ReplaceAllStringFunc(value, func(match string) string {
		ref := strings.TrimPrefix(match, "$")
		if ref == "$" {
			return "$"
		}
		name := strings.TrimSuffix(strings.TrimPrefix(ref, "{"), "}")
		v, ok := os.LookupEnv(name)
		if !ok || v == "" {
			if firstErr == nil {
				firstErr = &UnsetEnvError{Source: source, Field: field, Name: name}
			}
			return ""
		}
		return v
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

// expandEnvRef resolves a whole-string "$NAME" to the environment value; other
// strings pass through unchanged. Used where the env form is mandatory (auth
// secrets, SQL DSNs) so a literal can be rejected before expansion.
func expandEnvRef(source, field, value string) (string, error) {
	if !strings.HasPrefix(value, "$") {
		return value, nil
	}
	name := strings.TrimPrefix(value, "$")
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", &UnsetEnvError{Source: source, Field: field, Name: name}
	}
	return v, nil
}

// expandValueMap expands env references in header/query values, inline so
// "Bearer $TOKEN" and "$API_BASE/v2" both work.
func expandValueMap(source, field string, in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		expanded, err := expandEnvInline(source, field+"."+k, v)
		if err != nil {
			return nil, err
		}
		out[k] = expanded
	}
	return out, nil
}

// expandAuth validates and expands an auth block. Secret fields must be env
// references; a literal secret in the config file fails the build.
func expandAuth(source string, a AuthConfig) (AuthConfig, error) {
	switch a.Type {
	case "":
		return AuthConfig{}, nil
	case "bearer":
		return expandAuthField(source, a, "auth.token", a.Token, func(a *AuthConfig, v string) { a.Token = v })
	case "basic":
		out, err := expandAuthField(source, a, "auth.password", a.Password, func(a *AuthConfig, v string) { a.Password = v })
		if err != nil {
			return AuthConfig{}, err
		}
		if out.Username == "" {
			return AuthConfig{}, fmt.Errorf("external source %q: auth.username is required for basic auth", source)
		}
		return out, nil
	case "header":
		if a.Header == "" {
			return AuthConfig{}, fmt.Errorf("external source %q: auth.header is required for header auth", source)
		}
		return expandAuthField(source, a, "auth.value", a.Value, func(a *AuthConfig, v string) { a.Value = v })
	}
	return AuthConfig{}, fmt.Errorf("external source %q: unsupported auth.type %q (supported: bearer, basic, header)", source, a.Type)
}

// expandAuthField enforces the env-reference form for one secret field and
// stores the expanded value.
func expandAuthField(source string, a AuthConfig, field, value string, set func(*AuthConfig, string)) (AuthConfig, error) {
	if value == "" {
		return AuthConfig{}, fmt.Errorf("external source %q: %s is required for auth.type %q", source, field, a.Type)
	}
	if !strings.HasPrefix(value, "$") {
		return AuthConfig{}, fmt.Errorf("external source %q: %s must reference an environment variable (e.g. \"$API_TOKEN\"), not a literal secret", source, field)
	}
	expanded, err := expandEnvRef(source, field, value)
	if err != nil {
		return AuthConfig{}, err
	}
	set(&a, expanded)
	return a, nil
}
