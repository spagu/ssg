package config

import "strings"

// ResolveBasePath returns the path a site is served under, normalised to a
// leading slash and no trailing one ("/docs-site"), or "" when it is served
// from the root of its host (#306).
//
// An explicit base_path wins; "/" means the root even when `domain` carries a
// path. Without one the path of `domain` is used: `user.github.io/docs-site`
// already put that path into every canonical URL, the sitemap and robots.txt,
// while the links inside the pages still pointed at the host's root — a site
// that was half moved. Taking it from `domain` finishes the move.
func ResolveBasePath(explicit, domain string) string {
	if p := strings.TrimSpace(explicit); p != "" {
		return cleanBasePath(p)
	}
	host := strings.TrimSpace(domain)
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	_, path, found := strings.Cut(host, "/")
	if !found {
		return ""
	}
	return cleanBasePath(path)
}

// cleanBasePath trims slashes and whitespace and reattaches a single leading
// slash; a path that is only slashes is the root.
func cleanBasePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return "/" + p
}
