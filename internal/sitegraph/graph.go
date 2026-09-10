// Package sitegraph is one model of a published site, for anything that would
// otherwise scan directories to learn what the site contains (GO-095).
//
// The build already computes everything here — every page and its address,
// every taxonomy term, every redirect, every translation pair, and every link
// on every page (the link checker parses all of them, then throws the result
// away once it has validated them). It exposed that knowledge as four partial
// manifests grown one at a time: routes.json, search-index.json, llms.txt and
// sitemap.xml, each a different slice, each computed separately, each able to
// drift from the others.
//
// This package is the superset those views are generated FROM. An agent asks
// `site.pages()` or `site.links()`; a tool reads site-graph.json; the build
// derives routes.json and llms.txt from the same in-memory graph it writes —
// one truth, and a golden corpus that proves the derived files did not change
// by a byte when the source of truth moved.
//
// Public and internal are separated on purpose. Everything in the artifact is
// something the published HTML already reveals. What a page was rendered from
// is the project's structure, not the site's content, and stays in memory for
// the build's own views (json:"-").
package sitegraph

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Schema is the artifact's format version, bumped on an incompatible change so
// a reader can refuse a graph it does not understand rather than misread it.
const Schema = 1

// Build stamps a graph with what produced it, so a consumer can tell a fresh
// answer from a stale one.
type Build struct {
	Version string    `json:"version"`        // the ssg that ran
	Time    time.Time `json:"time"`           // when — the build's one clock read
	Hash    string    `json:"hash,omitempty"` // content hash of everything else; equal graphs hash equal
}

// Translation names the same document in another language.
type Translation struct {
	Lang string `json:"lang"`
	URL  string `json:"url"`
}

// Outputs are the addresses one document is published at, by format.
type Outputs struct {
	HTML     string `json:"html"`
	Markdown string `json:"markdown,omitempty"` // markdown_publish, absolute
	JSON     string `json:"json,omitempty"`     // outputs: [json]
}

// Page is one authored document as published.
type Page struct {
	URL         string     `json:"url"`
	Type        string     `json:"type"` // post | page
	Title       string     `json:"title"`
	Slug        string     `json:"slug,omitempty"`
	Lang        string     `json:"lang,omitempty"`
	Description string     `json:"description,omitempty"`
	Date        *time.Time `json:"date,omitempty"`
	Modified    *time.Time `json:"modified,omitempty"`
	Canonical   string     `json:"canonical"`
	Image       string     `json:"image,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Categories  []string   `json:"categories,omitempty"`
	Series      string     `json:"series,omitempty"`
	// Taxonomies is every assignment by taxonomy name, the built-ins included.
	Taxonomies   map[string][]string `json:"taxonomies,omitempty"`
	Translations []Translation       `json:"translations,omitempty"`
	// Relations are the links the author declared by name (GO-096) — the ones
	// no heuristic could have found.
	Relations   []Relation `json:"relations,omitempty"`
	Outputs     Outputs    `json:"outputs"`
	WordCount   int        `json:"word_count,omitempty"`
	ReadingTime int        `json:"reading_time,omitempty"`
	Sticky      bool       `json:"sticky,omitempty"`

	// Source is the file the page was rendered from. Internal: the build's own
	// views need it (routes.json publishes it, as it always has), the public
	// artifact does not carry it.
	Source string `json:"-"`
}

// Section is a generated listing: an archive, a taxonomy index, the post
// listing — a real URL that is not one authored document.
type Section struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"` // category | tag | series | author | <taxonomy> | <taxonomy>-index | listing
	Title string `json:"title,omitempty"`
	Lang  string `json:"lang,omitempty"`
}

// Term is one taxonomy value and where its archive is.
type Term struct {
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	URL   string `json:"url"`
	Lang  string `json:"lang,omitempty"`
	Count int    `json:"count"`
}

// Taxonomy is one classification with its terms.
type Taxonomy struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Path  string `json:"path"`
	Terms []Term `json:"terms"`
}

// Relation is one named link an author declared between two pages.
type Relation struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Link is one reference from a page to somewhere.
type Link struct {
	From string `json:"from"` // page URL
	To   string `json:"to"`   // site-relative path, or an absolute URL when external
	Kind string `json:"kind"` // page | asset | external
}

// Redirect is one rule the host applies before serving anything.
type Redirect struct {
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Status int    `json:"status"`
}

// Graph is the whole model.
type Graph struct {
	Schema     int        `json:"schema"`
	Build      Build      `json:"build"`
	Domain     string     `json:"domain"`
	Pages      []Page     `json:"pages"`
	Sections   []Section  `json:"sections"`
	Taxonomies []Taxonomy `json:"taxonomies"`
	Links      []Link     `json:"links"`
	Redirects  []Redirect `json:"redirects"`
}

// Encode renders the graph as indented JSON. The encoding is deterministic:
// the builder emits in a fixed order and the encoder adds none of its own, so
// two builds of the same site produce the same bytes — which is what makes
// the hash below mean something.
func Encode(g Graph) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Hash is a content hash over everything but the build stamp itself, so the
// same site built twice — a minute apart, on another machine — hashes equal,
// and a consumer comparing hashes learns whether the site changed, not
// whether the clock did.
func Hash(g Graph) (string, error) {
	g.Build = Build{Version: g.Build.Version}
	data, err := Encode(g)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8]), nil
}

// Stamp fills the build hash in place.
func (g *Graph) Stamp() error {
	h, err := Hash(*g)
	if err != nil {
		return err
	}
	g.Build.Hash = h
	return nil
}
