package deploy

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Canonical provider names accepted by --deploy.
const (
	ProviderCloudflare  = "cloudflare"
	ProviderGitHubPages = "github-pages"
	ProviderNetlify     = "netlify"
	ProviderVercel      = "vercel"
	ProviderFTP         = "ftp"
	ProviderSFTP        = "sftp"
)

// Options describes a deployment request. Secrets are never carried here — each
// provider reads its tokens/passwords/keys from Env (defaults to os.Getenv).
type Options struct {
	Provider string // cloudflare | github-pages | netlify | vercel | ftp | sftp
	Dir      string // directory to publish (the built output tree)
	Project  string // Pages/site/project name (provider-specific)
	Branch   string // target branch (cloudflare, github-pages)
	Target   string // ftp/sftp URL or git remote (provider-specific)
	Quiet    bool
	Env      func(string) string
}

// env returns an environment variable via the (possibly injected) lookup func.
func (o Options) env(key string) string {
	if o.Env != nil {
		return o.Env(key)
	}
	return os.Getenv(key)
}

func (o Options) logf(format string, args ...interface{}) {
	if !o.Quiet {
		fmt.Printf(format+"\n", args...)
	}
}

// SupportedProviders lists the deploy targets Run understands.
func SupportedProviders() []string {
	return []string{ProviderCloudflare, ProviderGitHubPages, ProviderNetlify, ProviderVercel, ProviderFTP, ProviderSFTP}
}

// canonicalProvider normalizes a provider name (lower-cased, aliases resolved) to its
// canonical form, or "" when unknown.
func canonicalProvider(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ProviderCloudflare, "cloudflare-pages":
		return ProviderCloudflare
	case ProviderGitHubPages, "github", "gh-pages":
		return ProviderGitHubPages
	case ProviderNetlify:
		return ProviderNetlify
	case ProviderVercel:
		return ProviderVercel
	case ProviderFTP:
		return ProviderFTP
	case ProviderSFTP, "ssh":
		return ProviderSFTP
	default:
		return ""
	}
}

// Supported reports whether name (including accepted aliases) is a known provider.
func Supported(name string) bool {
	return canonicalProvider(name) != ""
}

// Run publishes o.Dir to the selected provider and returns a human-facing URL (may be
// empty for providers that do not report one, e.g. FTP).
func Run(ctx context.Context, o Options) (string, error) {
	if o.Dir == "" {
		return "", fmt.Errorf("no output directory to deploy")
	}
	if info, err := os.Stat(o.Dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("output directory %q not found", o.Dir)
	}
	switch canonicalProvider(o.Provider) {
	case ProviderCloudflare:
		return deployCloudflare(ctx, o)
	case ProviderGitHubPages:
		return deployGitHubPages(ctx, o)
	case ProviderNetlify:
		return deployNetlify(ctx, o)
	case ProviderVercel:
		return deployVercel(ctx, o)
	case ProviderFTP:
		return deployFTP(ctx, o)
	case ProviderSFTP:
		return deploySFTP(ctx, o)
	default:
		return "", fmt.Errorf("unknown deploy provider %q (want one of: %s)",
			o.Provider, strings.Join(SupportedProviders(), ", "))
	}
}

// parseDeployURL parses a deploy target URL, validates its scheme and ensures the
// host carries a port (applying defaultPort when omitted).
func parseDeployURL(target, scheme string, defaultPort int) (*url.URL, error) {
	if target == "" {
		return nil, fmt.Errorf("%s deploy needs --deploy-target=%s://[user@]host[:port]/path", scheme, scheme)
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid --deploy-target %q: %w", target, err)
	}
	if u.Scheme != scheme {
		return nil, fmt.Errorf("--deploy-target must use the %q scheme, got %q", scheme, u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("--deploy-target %q has no host", target)
	}
	if u.Port() == "" {
		u.Host = fmt.Sprintf("%s:%d", u.Hostname(), defaultPort)
	}
	return u, nil
}

// credentials resolves a username/password from URL userinfo first, then env vars.
func (o Options) credentials(u *url.URL, userEnv, passEnv string) (user, pass string) {
	if u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	}
	if user == "" {
		user = o.env(userEnv)
	}
	if pass == "" {
		pass = o.env(passEnv)
	}
	return user, pass
}

// localFile is one file in the output tree with its relative path and bytes.
type localFile struct {
	Rel  string // forward-slash path relative to the root, no leading slash
	Data []byte
}

// walkFiles reads every regular file under dir into memory with a normalized,
// forward-slash relative path. Sites are small static trees, so buffering is fine.
func walkFiles(dir string) ([]localFile, error) {
	var files []localFile
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304,G122 -- deploy reads the CLI's own output tree; path from local Walk
		if err != nil {
			return err
		}
		files = append(files, localFile{Rel: filepath.ToSlash(rel), Data: data})
		return nil
	})
	return files, err
}
