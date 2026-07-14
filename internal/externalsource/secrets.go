package externalsource

import (
	"fmt"
	"os"
	"strings"
)

// Secrets come exclusively from environment variables (plan §Jeden system
// sekretów): a value of "$NAME" resolves to os.Getenv("NAME"). Auth secret
// fields REQUIRE the env form so credentials never live in the config file,
// and error messages only ever name the variable, never its value.

// expandEnvRef resolves "$NAME" to the environment value; other strings pass
// through unchanged. A referenced-but-unset variable is an error naming NAME.
func expandEnvRef(source, field, value string) (string, error) {
	if !strings.HasPrefix(value, "$") {
		return value, nil
	}
	name := strings.TrimPrefix(value, "$")
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", fmt.Errorf("external source %q: %s references $%s, which is not set in the environment", source, field, name)
	}
	return v, nil
}

// expandValueMap expands env references in header/query values.
func expandValueMap(source, field string, in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		expanded, err := expandEnvRef(source, field+"."+k, v)
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
