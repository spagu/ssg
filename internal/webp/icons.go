package webp

// Platform icons stay in the format they were published in (#392).
//
// iOS and iPadOS accept only PNG for a home-screen icon, older Safari ignores a
// WebP favicon, and a web app manifest is JSON that the reference pass never
// reads. Each consumer fetches the icon by the URL it was given. Converting the
// file breaks it in two ways: replace mode deletes the PNG it points at, and
// the reference pass rewrites <link rel="apple-touch-icon"> to a .webp that iOS
// will not use. So an icon is neither converted nor re-pointed. The share
// preview metas follow the same rule under webp_keep_original (#385).

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// linkTagRe captures whole <link> tags; linkRelRe and linkHrefRe read them.
var (
	linkTagRe  = regexp.MustCompile(`(?i)<link\b[^>]*>`)
	linkRelRe  = regexp.MustCompile(`(?i)\brel\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	linkHrefRe = regexp.MustCompile(`(?i)\bhref\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

// rootManifests are the names a web app manifest goes by at the site root,
// where browsers look for it even when no page links it.
var rootManifests = []string{"manifest.webmanifest", "site.webmanifest", "manifest.json"}

// iconRel reports whether a rel value names a platform icon: icon, shortcut
// icon, apple-touch-icon(-precomposed), mask-icon. A manifest is not an icon
// itself; the icons it lists are collected separately.
func iconRel(rel string) bool {
	for _, token := range strings.Fields(strings.ToLower(rel)) {
		switch token {
		case "icon", "apple-touch-icon", "apple-touch-icon-precomposed", "mask-icon":
			return true
		}
	}
	return false
}

// unquote strips the quotes an attribute value may carry.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// iconSet is the set of icon files under one output root.
type iconSet struct {
	root, origin string
	files        map[string]bool // cleaned absolute paths
	manifests    map[string]bool
}

// add records ref, as written in a file under baseDir, when it is local.
func (s iconSet) add(ref, baseDir string, into map[string]bool) {
	if s.origin != "" && strings.HasPrefix(ref, s.origin+"/") {
		ref = strings.TrimPrefix(ref, s.origin)
	}
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" || isRemoteURL(ref) || strings.Contains(ref, ":") {
		return // another host, or data: and friends
	}
	if strings.HasPrefix(ref, "/") {
		into[filepath.Clean(filepath.Join(s.root, filepath.FromSlash(path.Clean(ref))))] = true
		return
	}
	into[filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(ref)))] = true
}

// scanHTML records the icons and manifests one page links.
func (s iconSet) scanHTML(file string, content []byte) {
	for _, tag := range linkTagRe.FindAll(content, -1) {
		rel, href := linkRelRe.FindSubmatch(tag), linkHrefRe.FindSubmatch(tag)
		if rel == nil || href == nil {
			continue
		}
		value := strings.TrimSpace(unquote(string(href[1])))
		switch r := unquote(string(rel[1])); {
		case iconRel(r):
			s.add(value, filepath.Dir(file), s.files)
		case strings.EqualFold(strings.TrimSpace(r), "manifest"):
			s.add(value, filepath.Dir(file), s.manifests)
		}
	}
}

// scanManifest records the icons a web app manifest lists. Its src values
// resolve against the manifest's own URL, as the spec says.
func (s iconSet) scanManifest(file string) {
	data, err := os.ReadFile(file) // #nosec G304 -- a file in the build's own output
	if err != nil {
		return // linked but not published: nothing to keep
	}
	var m struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	for _, icon := range m.Icons {
		s.add(strings.TrimSpace(icon.Src), filepath.Dir(file), s.files)
	}
}

// platformIcons lists the icon files of the site under dir: those its pages
// link as icons and those its web app manifests list. origin
// ("https://example.com") marks the site's own absolute URLs as local.
func platformIcons(dir, origin string) map[string]bool {
	s := iconSet{root: dir, origin: strings.TrimRight(origin, "/"), files: map[string]bool{}, manifests: map[string]bool{}}
	for _, name := range rootManifests {
		s.manifests[filepath.Join(dir, name)] = true
	}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(p), ".html") {
			return nil //nolint:nilerr // an unreadable page links no icon worth failing over
		}
		if content, readErr := os.ReadFile(p); readErr == nil { // #nosec G304,G122 -- the build's own output
			s.scanHTML(p, content)
		}
		return nil
	})
	for m := range s.manifests {
		s.scanManifest(m)
	}
	return s.files
}
