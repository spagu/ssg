package generator

// Story mode (1.9.4): putting the presentation on a page. The page gets its
// settings as JSON (<script type="application/json" id="ssg-story">), the
// stylesheets in <head> and the scripts before </body>; the scripts and
// styles themselves are written once per build under /assets/ssg-story/,
// with a content hash in the query so a new ssg is never served stale.
//
// The narrator's pronunciation rules come from the built-in set for the
// page's language (assets/story/pronounce/<lang>.json), with the site's and
// the post's own `narrator.pronounce` over them; a rule set to null is
// dropped, `narrator.pronounce_defaults: false` drops the whole set.

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

//go:embed assets/story
var storyFS embed.FS

const (
	storyAssetsDir = "assets/ssg-story" // under the output, and the site path
	storyDataID    = "ssg-story"
)

// storyScripts load in this order: each adds to window.ssgStory.
var storyScripts = []string{"story-recipe.js", "story.js", "story-show.js", "story-say.js",
	"story-voice.js", "story-touch.js", "story-share.js", "story-track.js"}

// storyStyles are linked in <head>.
var storyStyles = []string{"story.css", "story-share.css"}

// storyFiles is every script and stylesheet a page loads, by name, read
// from the binary once.
var storyFiles = func() map[string][]byte {
	out := map[string][]byte{}
	for _, f := range append(append([]string{}, storyScripts...), storyStyles...) {
		data, err := storyFS.ReadFile(path.Join("assets/story", f))
		if err != nil {
			panic("story asset missing from the binary: " + f) // a build mistake, caught by every test
		}
		out[f] = data
	}
	return out
}()

// storyAssetVersion is a short hash of the scripts and styles.
var storyAssetVersion = func() string {
	names := make([]string, 0, len(storyFiles))
	for f := range storyFiles {
		names = append(names, f)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, f := range names {
		h.Write([]byte(f))
		h.Write(storyFiles[f])
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}()

// storyHTMLString adds the presentation to a page that has one.
func (g *Generator) storyHTMLString(s string, page *models.Page, isPost bool) string {
	if page == nil || page.Content == "" || isAMPDocument(s) || strings.Contains(s, `id="`+storyDataID+`"`) {
		return s
	}
	settings, ok := g.storySettings(*page, isPost)
	if !ok {
		return s
	}
	g.storyFinish(settings, *page)
	data, err := json.Marshal(settings)
	if err != nil {
		g.log("   ⚠️  story: " + page.GetURL() + ": " + err.Error())
		return s
	}
	g.storyUsed.Store(true)
	base := withBasePath("/"+storyAssetsDir+"/", g.config.BasePath)
	var head, tail strings.Builder
	for _, f := range storyStyles {
		head.WriteString(`<link rel="stylesheet" href="` + base + f + `?v=` + storyAssetVersion + `" data-ssg-story-asset>`)
	}
	// "</" cannot end the JSON's <script> early.
	tail.WriteString(`<script type="application/json" id="` + storyDataID + `">` +
		strings.ReplaceAll(string(data), "</", `<\/`) + `</script>`)
	for _, f := range storyScripts {
		tail.WriteString(`<script src="` + base + f + `?v=` + storyAssetVersion + `" defer data-ssg-story-asset></script>`)
	}
	if i := strings.Index(s, "</head>"); i >= 0 {
		s = s[:i] + head.String() + s[i:]
	} else {
		s = head.String() + s
	}
	return insertBeforeBody(s, tail.String())
}

// storyFinish adds what only the build knows: the pronunciation rules and
// the link to the page's Web Story.
func (g *Generator) storyFinish(settings map[string]any, page models.Page) {
	narrator, _ := settings["narrator"].(map[string]any)
	if narrator == nil {
		narrator = map[string]any{}
	}
	if rules := storyPronounce(narrator, g.storyLang(page)); len(rules) > 0 {
		narrator["pronounce"] = rules
	} else {
		delete(narrator, "pronounce")
	}
	delete(narrator, "pronounceDefaults")
	settings["narrator"] = narrator
	if href := g.webStories.byPost[page.GetURL()]; href != "" {
		share, _ := settings["share"].(map[string]any)
		if share == nil {
			share = map[string]any{}
		}
		share["webStory"] = withBasePath(href, g.config.BasePath)
		settings["share"] = share
	}
}

// storyPronounce is the built-in set for lang with narrator.pronounce over it.
func storyPronounce(narrator map[string]any, lang string) map[string]any {
	rules := map[string]any{}
	if on, ok := narrator["pronounceDefaults"].(bool); !ok || on {
		for k, v := range builtinPronounce(lang) {
			rules[k] = v
		}
	}
	own, _ := narrator["pronounce"].(map[string]any)
	for k, v := range own {
		if v == nil {
			delete(rules, k)
		} else {
			rules[k] = v
		}
	}
	return rules
}

// pronounceSet is one assets/story/pronounce/<lang>.json.
type pronounceSet struct {
	Lang     string            `json:"lang"`
	Rules    map[string]string `json:"rules"`
	Examples [][2]string       `json:"examples"`
}

// pronounceSets lists the built-in sets by name (pl_PL, en_GB, …).
func pronounceSets() []string {
	entries, _ := storyFS.ReadDir("assets/story/pronounce")
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(names)
	return names
}

// loadPronounceSet reads one built-in set.
func loadPronounceSet(name string) (pronounceSet, error) {
	var set pronounceSet
	data, err := storyFS.ReadFile("assets/story/pronounce/" + name + ".json")
	if err == nil {
		err = json.Unmarshal(data, &set)
	}
	return set, err
}

// builtinPronounce is the set for a language: pl-PL takes pl_PL, else the
// first set of the same language (pl_*); none is nil.
func builtinPronounce(lang string) map[string]string {
	lang = strings.ReplaceAll(strings.TrimSpace(lang), "-", "_")
	names := pronounceSets()
	pick := ""
	for _, n := range names {
		if strings.EqualFold(n, lang) {
			pick = n
		}
	}
	if pick == "" {
		base := strings.ToLower(strings.SplitN(lang, "_", 2)[0])
		for _, n := range names {
			if pick == "" && strings.ToLower(strings.SplitN(n, "_", 2)[0]) == base {
				pick = n
			}
		}
	}
	if pick == "" {
		return nil
	}
	set, _ := loadPronounceSet(pick) // embedded and tested: cannot fail at run time
	return set.Rules
}

// writeStoryAssets writes the scripts and styles once, when a page used them.
func (g *Generator) writeStoryAssets() error {
	if !g.storyUsed.Load() {
		return nil
	}
	dir := filepath.Join(g.config.OutputDir, filepath.FromSlash(storyAssetsDir))
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- public web assets
		return err
	}
	for f, data := range storyFiles {
		// #nosec G306 -- web content files need to be world-readable
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
