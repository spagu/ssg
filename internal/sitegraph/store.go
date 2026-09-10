package sitegraph

// Writing and reading the artifact.
//
// A small site is one file, site-graph.json, readable by anything. A large one
// — the threshold is pages, the thing that grows — keeps site-graph.json as an
// INDEX naming shards, and puts pages and links in JSON Lines under
// site-graph/, so a consumer can stream one section without loading fifty
// thousand records to find it. Load hides the difference.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the artifact at the site root.
const FileName = "site-graph.json"

// ShardDir holds the JSON Lines shards of a large graph.
const ShardDir = "site-graph"

// DefaultShardAbove is the page count past which the graph is sharded; the
// shard size keeps each file comfortably streamable.
const (
	DefaultShardAbove = 10000
	shardSize         = 5000
)

// index is what site-graph.json holds when the graph is sharded: everything
// small inline, and the names of the files holding the rest.
type index struct {
	Schema     int        `json:"schema"`
	Build      Build      `json:"build"`
	Domain     string     `json:"domain"`
	Sharded    bool       `json:"sharded"`
	Counts     counts     `json:"counts"`
	Shards     shards     `json:"shards"`
	Sections   []Section  `json:"sections"`
	Taxonomies []Taxonomy `json:"taxonomies"`
	Redirects  []Redirect `json:"redirects"`
}

type counts struct {
	Pages int `json:"pages"`
	Links int `json:"links"`
}

type shards struct {
	Pages []string `json:"pages"`
	Links string   `json:"links"`
}

// Write stores the graph under dir and returns the files it wrote, relative to
// dir. shardAbove <= 0 means the default threshold.
func Write(dir string, g Graph, shardAbove int) ([]string, error) {
	if shardAbove <= 0 {
		shardAbove = DefaultShardAbove
	}
	if len(g.Pages) <= shardAbove {
		data, err := Encode(g)
		if err != nil {
			return nil, err
		}
		return []string{FileName}, writeFile(filepath.Join(dir, FileName), data)
	}
	return writeSharded(dir, g)
}

func writeSharded(dir string, g Graph) ([]string, error) {
	shardPath := filepath.Join(dir, ShardDir)
	if err := os.MkdirAll(shardPath, 0o755); err != nil { // #nosec G301 -- a public output dir, browsable like the HTML beside it
		return nil, err
	}
	var written []string
	idx := index{
		Schema: g.Schema, Build: g.Build, Domain: g.Domain, Sharded: true,
		Counts:   counts{Pages: len(g.Pages), Links: len(g.Links)},
		Sections: g.Sections, Taxonomies: g.Taxonomies, Redirects: g.Redirects,
	}
	for i := 0; i*shardSize < len(g.Pages); i++ {
		end := (i + 1) * shardSize
		if end > len(g.Pages) {
			end = len(g.Pages)
		}
		name := filepath.ToSlash(filepath.Join(ShardDir, fmt.Sprintf("pages-%d.jsonl", i+1)))
		if err := writeLines(filepath.Join(dir, filepath.FromSlash(name)), g.Pages[i:end]); err != nil {
			return nil, err
		}
		idx.Shards.Pages = append(idx.Shards.Pages, name)
		written = append(written, name)
	}
	links := filepath.ToSlash(filepath.Join(ShardDir, "links.jsonl"))
	if err := writeLines(filepath.Join(dir, filepath.FromSlash(links)), g.Links); err != nil {
		return nil, err
	}
	idx.Shards.Links = links
	written = append(written, links)

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]string{FileName}, written...), writeFile(filepath.Join(dir, FileName), append(data, '\n'))
}

// writeLines writes one JSON document per line.
func writeLines[T any](path string, items []T) error {
	f, err := os.Create(path) // #nosec G304 -- build writes into its own output tree
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeFile(path string, data []byte) error {
	// #nosec G306 -- a public build artifact, world-readable like the HTML beside it
	return os.WriteFile(path, data, 0o644)
}

// ErrNotFound reports that dir holds no graph.
var ErrNotFound = errors.New("sitegraph: no " + FileName + " — build with site_graph: true")

// Load reads a graph back from dir, sharded or not.
func Load(dir string) (Graph, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName)) // #nosec G304 -- reads the build's own artifact
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Graph{}, ErrNotFound
		}
		return Graph{}, err
	}
	var probe struct {
		Schema  int  `json:"schema"`
		Sharded bool `json:"sharded"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Graph{}, fmt.Errorf("sitegraph: %s: %w", FileName, err)
	}
	if probe.Schema != Schema {
		return Graph{}, fmt.Errorf("sitegraph: schema %d, this build reads %d", probe.Schema, Schema)
	}
	if !probe.Sharded {
		var g Graph
		return g, json.Unmarshal(data, &g)
	}
	var idx index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Graph{}, err
	}
	g := Graph{Schema: idx.Schema, Build: idx.Build, Domain: idx.Domain,
		Sections: idx.Sections, Taxonomies: idx.Taxonomies, Redirects: idx.Redirects}
	for _, name := range idx.Shards.Pages {
		pages, err := readLines[Page](filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return Graph{}, err
		}
		g.Pages = append(g.Pages, pages...)
	}
	links, err := readLines[Link](filepath.Join(dir, filepath.FromSlash(idx.Shards.Links)))
	if err != nil {
		return Graph{}, err
	}
	g.Links = links
	return g, nil
}

func readLines[T any](path string) ([]T, error) {
	f, err := os.Open(path) // #nosec G304 -- reads the build's own artifact
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var it T
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, fmt.Errorf("sitegraph: %s: %w", filepath.Base(path), err)
		}
		out = append(out, it)
	}
	return out, sc.Err()
}
