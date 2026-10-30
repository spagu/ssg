package generator

// The static half of Web Push (#368).
//
// A static site cannot send notifications: someone has to keep the
// subscriptions and sign every message with a private key. The web-push worker
// does that, and ships the service worker and the subscribe script in its
// public/. What the pages need besides is a web app manifest — iOS delivers
// push only to a site added to the home screen — and the script in <head>,
// so a theme has nothing to wire but the button.

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"os"
	"path/filepath"
	"strings"
)

// webManifestName is the manifest file at the site root.
const webManifestName = "manifest.webmanifest"

// WebPushOptions configures the static half. The zero value does nothing.
type WebPushOptions struct {
	Enabled  bool
	Endpoint string // default "/api/push"
}

// webPushHead is the manifest link and the subscribe script, skipping what the
// theme already has.
func (g *Generator) webPushHead(existing string) string {
	var b strings.Builder
	if !strings.Contains(existing, `rel="manifest"`) {
		fmt.Fprintf(&b, `<link rel="manifest" href="%s">`+"\n",
			stdhtml.EscapeString(withBasePath("/"+webManifestName, g.config.BasePath)))
	}
	if !strings.Contains(existing, "push.js") {
		fmt.Fprintf(&b, `<script src="%s" defer data-endpoint="%s"></script>`+"\n",
			stdhtml.EscapeString(withBasePath("/push.js", g.config.BasePath)),
			stdhtml.EscapeString(firstNonEmpty(g.config.WebPush.Endpoint, "/api/push")))
	}
	return b.String()
}

// injectWebPush places webPushHead before </head>.
func (g *Generator) injectWebPush(s string) string {
	if !g.config.WebPush.Enabled {
		return s
	}
	i := strings.LastIndex(s, "</head>")
	if i < 0 {
		return s
	}
	return s[:i] + g.webPushHead(s[:i]) + s[i:]
}

// webManifest is the subset of the Web App Manifest a push-capable site needs.
type webManifest struct {
	Name       string         `json:"name"`
	ShortName  string         `json:"short_name"`
	StartURL   string         `json:"start_url"`
	Scope      string         `json:"scope"`
	Display    string         `json:"display"`
	ThemeColor string         `json:"theme_color,omitempty"`
	Icons      []manifestIcon `json:"icons,omitempty"`
}

type manifestIcon struct {
	Src string `json:"src"`
}

// writeWebPush writes manifest.webmanifest unless the site ships its own, and
// warns when the worker's scripts are not in the output: without them the
// injected script tag points at nothing.
func (g *Generator) writeWebPush() error {
	if !g.config.WebPush.Enabled {
		return nil
	}
	for _, name := range []string{"push.js", "sw.js"} {
		if _, err := os.Stat(filepath.Join(g.config.OutputDir, name)); err != nil {
			fmt.Printf("   ⚠️  web_push: /%s is not in the output — add the worker with `ssg new worker web-push`\n", name)
		}
	}
	path := filepath.Join(g.config.OutputDir, webManifestName)
	if _, err := os.Stat(path); err == nil {
		return nil // the site's own, from static/ or the theme
	}
	// Strings and a slice of them: encoding cannot fail.
	data, _ := json.MarshalIndent(g.webManifest(), "", "  ")
	// #nosec G306 -- a public file served with the site
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// webManifest describes the site from what it already declares.
func (g *Generator) webManifest() webManifest {
	name := firstNonEmpty(g.siteData.Title, g.config.Domain)
	root := withBasePath("/", g.config.BasePath)
	m := webManifest{
		Name: name, ShortName: name, StartURL: root, Scope: root,
		Display: "standalone", ThemeColor: g.siteData.Marketing.ThemeColor,
	}
	for _, icon := range []string{g.siteData.Marketing.AppleTouchIcon, g.siteData.Marketing.Favicon} {
		if icon != "" {
			m.Icons = append(m.Icons, manifestIcon{Src: icon})
		}
	}
	return m
}
