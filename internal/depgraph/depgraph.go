package depgraph

// What a build has to redo when one file changes (GO-094).
//
// The watch loop has had one increment of this since PLAT-006: it hashes the
// content tree and skips a rebuild when nothing changed by a byte. Every real
// change then costs a full build — every page, every archive, every aggregate.
// At five thousand posts that is a second and a half; at fifty thousand it is
// most of a minute, and it happens on every keystroke's worth of saved work.
//
// A dependency graph answers the narrower question: given these changed files,
// which outputs can possibly differ? Everything else is already on disk and
// correct.
//
// The rule the whole design rests on is the one PLAT-006 wrote down and this
// inherits: **an uncertain dependency means rebuild everything.** A graph that
// misses an edge produces a stale page with a green build, which is exactly the
// class of silent failure the 1.8.55–1.8.59 week was made of. Being wrong here
// is worse than being slow, so every rule below fails towards the full build.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Schema versions the persisted graph. A graph written by another version is
// not read — it is discarded and the build is full, which is the safe answer.
const Schema = 1

// FileName is the persisted graph, under the shared cache root (GO-091).
const FileName = "graph.json"

// Kind labels a node, so a plan can explain itself.
type Kind string

// The kinds of thing a build depends on.
const (
	KindContent  Kind = "content"  // a Markdown file
	KindTemplate Kind = "template" // a theme template or partial
	KindData     Kind = "data"     // a file under data/
	KindConfig   Kind = "config"   // the site configuration
	KindOutput   Kind = "output"   // a file the build wrote
)

// Node is one thing the build read or wrote.
type Node struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	// Hash is the content hash of an input, empty for an output.
	Hash string `json:"hash,omitempty"`
}

// Graph is the dependency graph of one build.
//
// Edges point from an input to what it affects, which is the direction a plan
// walks: "this file changed, so these outputs must be rewritten".
type Graph struct {
	Schema int                 `json:"schema"`
	Nodes  map[string]Node     `json:"nodes"`
	Edges  map[string][]string `json:"edges"`
	// Global records that the build contained something whose effect could not
	// be narrowed — a template using site-wide data, a config the graph does
	// not model. A global build's plan is always full.
	Global []string `json:"global,omitempty"`

	mu sync.Mutex
}

// New starts an empty graph.
func New() *Graph {
	return &Graph{Schema: Schema, Nodes: map[string]Node{}, Edges: map[string][]string{}}
}

// AddInput records a file the build read, with the hash it had.
func (g *Graph) AddInput(id string, kind Kind, hash string) {
	if g == nil || id == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Nodes[id] = Node{ID: id, Kind: kind, Hash: hash}
}

// AddOutput records a file the build wrote.
func (g *Graph) AddOutput(id string) {
	if g == nil || id == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.Nodes[id]; !ok {
		g.Nodes[id] = Node{ID: id, Kind: KindOutput}
	}
}

// Depends records that output depends on input.
func (g *Graph) Depends(output, input string) {
	if g == nil || output == "" || input == "" {
		return
	}
	g.AddOutput(output)
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, existing := range g.Edges[input] {
		if existing == output {
			return
		}
	}
	g.Edges[input] = append(g.Edges[input], output)
}

// MarkGlobal records something whose effect the graph cannot narrow, with the
// reason — which `ssg graph` prints, because "your build is never incremental"
// is useless without "and here is why".
func (g *Graph) MarkGlobal(reason string) {
	if g == nil || reason == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, existing := range g.Global {
		if existing == reason {
			return
		}
	}
	g.Global = append(g.Global, reason)
	sort.Strings(g.Global)
}

// IsGlobal reports whether this build could be narrowed at all.
func (g *Graph) IsGlobal() bool {
	if g == nil {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.Global) > 0
}

// Reasons lists why a build is global.
func (g *Graph) Reasons() []string {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.Global...)
}

// Plan is what a change means for the next build.
type Plan struct {
	// Full says the build cannot be narrowed and must run whole.
	Full bool
	// Reason explains the plan in one line, for `ssg graph` and for the log.
	Reason string
	// Outputs are the files that must be rewritten, when Full is false.
	Outputs []string
	// Changed are the inputs that differ from the recorded graph.
	Changed []string
}

// PlanFor works out what has to be redone, given the files that changed.
//
// It answers Full whenever it is not certain, and the list of cases where it is
// not certain is deliberately long: no previous graph, a graph from another
// schema, a global build, a changed file the graph never saw, a changed
// template, a changed config. What remains — a changed content or data file the
// previous build read — is the common case in a watch loop and the one worth
// getting right.
func (g *Graph) PlanFor(changed []string) Plan {
	if g == nil || g.Schema != Schema || len(g.Nodes) == 0 {
		return Plan{Full: true, Reason: "no usable graph from a previous build"}
	}
	if reasons := g.Reasons(); len(reasons) > 0 {
		return Plan{Full: true, Reason: "this site's build cannot be narrowed: " + strings.Join(reasons, "; ")}
	}
	if len(changed) == 0 {
		return Plan{Reason: "nothing changed"}
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	seen := map[string]bool{}
	var outputs []string
	for _, id := range changed {
		node, known := g.Nodes[id]
		if !known {
			return Plan{Full: true, Changed: changed,
				Reason: fmt.Sprintf("%s is new to this build, and what depends on it is unknown", id)}
		}
		switch node.Kind {
		case KindTemplate:
			return Plan{Full: true, Changed: changed,
				Reason: fmt.Sprintf("%s is a template; every page rendered through it may differ", id)}
		case KindConfig:
			return Plan{Full: true, Changed: changed,
				Reason: fmt.Sprintf("%s is the configuration, which can change anything", id)}
		case KindOutput:
			return Plan{Full: true, Changed: changed,
				Reason: fmt.Sprintf("%s is an output of the build, not an input", id)}
		}
		for _, out := range g.Edges[id] {
			if !seen[out] {
				seen[out] = true
				outputs = append(outputs, out)
			}
		}
	}
	sort.Strings(outputs)
	return Plan{
		Outputs: outputs, Changed: changed,
		Reason: fmt.Sprintf("%d changed file(s) affect %d output(s)", len(changed), len(outputs)),
	}
}

// ChangedSince compares the recorded hashes with the ones on disk now, and
// reports which inputs differ. A file the graph recorded and that is now
// missing counts as changed.
func (g *Graph) ChangedSince(hash func(path string) (string, bool)) []string {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var changed []string
	for id, node := range g.Nodes {
		if node.Kind == KindOutput {
			continue
		}
		now, ok := hash(id)
		if !ok || now != node.Hash {
			changed = append(changed, id)
		}
	}
	sort.Strings(changed)
	return changed
}

// Explain describes what one input pulls with it, for `ssg graph`.
func (g *Graph) Explain(id string) Plan {
	plan := g.PlanFor([]string{id})
	if plan.Full {
		return plan
	}
	return plan
}

// HashBytes is the content hash the graph stores.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HashFile hashes a file, reporting whether it could be read.
func HashFile(path string) (string, bool) {
	data, err := os.ReadFile(path) // #nosec G304 -- the build's own inputs
	if err != nil {
		return "", false
	}
	return HashBytes(data), true
}

// Save writes the graph under dir.
func (g *Graph) Save(dir string) error {
	if g == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	g.mu.Lock()
	data, err := json.MarshalIndent(struct {
		Schema int                 `json:"schema"`
		Nodes  map[string]Node     `json:"nodes"`
		Edges  map[string][]string `json:"edges"`
		Global []string            `json:"global,omitempty"`
	}{g.Schema, g.Nodes, g.Edges, g.Global}, "", "  ")
	g.mu.Unlock()
	if err != nil {
		return err
	}
	// #nosec G306 -- a cache file beside the project's other caches
	return os.WriteFile(filepath.Join(dir, FileName), append(data, '\n'), 0o644)
}

// Load reads a graph from dir. A missing, unreadable or foreign-schema graph is
// not an error: it means the next build is full, which is always allowed.
func Load(dir string) *Graph {
	data, err := os.ReadFile(filepath.Join(dir, FileName)) // #nosec G304 -- the build's own cache
	if err != nil {
		return nil
	}
	var raw struct {
		Schema int                 `json:"schema"`
		Nodes  map[string]Node     `json:"nodes"`
		Edges  map[string][]string `json:"edges"`
		Global []string            `json:"global,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Schema != Schema {
		return nil
	}
	return &Graph{Schema: raw.Schema, Nodes: raw.Nodes, Edges: raw.Edges, Global: raw.Global}
}

// InputIDs lists every file the build read, so two builds' input sets can be
// compared — a file that appeared since the last build is one whose effects the
// previous graph cannot describe.
func (g *Graph) InputIDs() []string {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.Nodes))
	for id, n := range g.Nodes {
		if n.Kind != KindOutput {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Knows reports whether the graph recorded this file at all.
func (g *Graph) Knows(id string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.Nodes[id]
	return ok
}

// HasOutput reports whether the graph recorded this output.
func (g *Graph) HasOutput(id string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	n, ok := g.Nodes[id]
	return ok && n.Kind == KindOutput
}

// Stats describes a graph for `ssg graph` and the log.
type Stats struct {
	Inputs  int
	Outputs int
	Edges   int
}

// Stats counts what the graph holds.
func (g *Graph) Stats() Stats {
	if g == nil {
		return Stats{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var s Stats
	for _, n := range g.Nodes {
		if n.Kind == KindOutput {
			s.Outputs++
		} else {
			s.Inputs++
		}
	}
	for _, outs := range g.Edges {
		s.Edges += len(outs)
	}
	return s
}

// DOT renders the graph for `dot -Tsvg`, which is how a fan-out is actually
// seen rather than counted.
func (g *Graph) DOT() string {
	var b strings.Builder
	b.WriteString("digraph ssg {\n  rankdir=LR;\n  node [shape=box,fontname=\"sans-serif\"];\n")
	if g == nil {
		b.WriteString("}\n")
		return b.String()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	inputs := make([]string, 0, len(g.Edges))
	for id := range g.Edges {
		inputs = append(inputs, id)
	}
	sort.Strings(inputs)
	for _, id := range inputs {
		for _, out := range g.Edges[id] {
			fmt.Fprintf(&b, "  %q -> %q;\n", id, out)
		}
	}
	b.WriteString("}\n")
	return b.String()
}
