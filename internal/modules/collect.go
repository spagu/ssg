package modules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Finding is one problem the build reports under check_modules: the file it
// is in (a module.json, or the site's configuration) and what is wrong,
// naming the key.
type Finding struct {
	File   string
	Detail string
}

// Worker is one entry of `workers:` as the build resolved it: Key names it in
// messages ("workers[0]"), Dir is where its files are.
type Worker struct {
	Key string
	Dir string
}

// Input is everything Collect reads.
type Input struct {
	// ConfigFile names the site's configuration in messages.
	ConfigFile string
	Workers    []Worker
	Refs       []Ref
	Schema     *Schema
	// Templates holds the templates this binary ships, under workers/.
	Templates fs.FS
	// Theme is the computed `admin_theme:` (GO-152), or nil; Render puts it
	// in the registry as it is.
	Theme *Theme
}

// sourced is an entry with what messages need to say where it came from.
type sourced struct {
	key   string
	panel *string
	entry Entry
}

type collector struct {
	in       Input
	entries  []sourced
	findings []Finding
}

// Collect assembles the registry from the manifests of the workers the build
// bundles and the `modules:` entries, and lists every problem it met. A
// module whose manifest is broken is left out (bundled) or kept without a
// manifest (declared in `modules:`, which core can still reach); the rest of
// the registry is unaffected.
func Collect(in Input) (Registry, []Finding) {
	c := &collector{in: in}
	for _, w := range in.Workers {
		c.worker(w)
	}
	for i, r := range in.Refs {
		c.ref(fmt.Sprintf("modules[%d]", i), r)
	}
	kept := c.dedupe()
	c.checkPanels(kept)
	c.checkBases(kept)
	reg := Registry{Format: RegistryFormat, Available: available(in.Templates, in.Schema, kept)}
	for _, s := range kept {
		reg.Modules = append(reg.Modules, s.entry)
	}
	return reg, c.findings
}

func (c *collector) report(file, format string, args ...any) {
	c.findings = append(c.findings, Finding{File: file, Detail: fmt.Sprintf(format, args...)})
}

// read loads dir/module.json. found is false when there is none.
func (c *collector) read(dir string) (m *Manifest, file string, found bool) {
	file = filepath.Join(dir, "module.json")
	data, err := os.ReadFile(file) // #nosec G304 -- a worker dir from the project's own config
	if errors.Is(err, fs.ErrNotExist) {
		return nil, file, false
	}
	if err != nil {
		c.report(file, "%v", err)
		return nil, file, true
	}
	m, errs := c.in.Schema.ReadManifest(data)
	for _, e := range errs {
		c.report(file, "%s", e)
	}
	return m, file, true
}

// worker registers a Pages Function this build bundles. A worker without a
// module.json is not a module and is simply not listed.
func (c *collector) worker(w Worker) {
	m, file, found := c.read(w.Dir)
	if !found {
		return
	}
	if m == nil {
		c.report(file, "%s is left out of the module registry until its manifest is fixed", w.Key)
		return
	}
	if m.Kind == KindWorker {
		c.report(file, "manifest.kind is %q, a standalone Worker, but %s builds it into this site's functions/ — deploy it on its own and list it under modules:", KindWorker, w.Key)
	}
	c.entries = append(c.entries, sourced{key: w.Key, panel: m.Panel, entry: entryFor(m, "/", true)})
}

// ref registers a module deployed outside this build.
func (c *collector) ref(key string, r Ref) {
	s := sourced{key: key, entry: Entry{ID: r.ID, Base: cleanBase(r.URL)}}
	if r.Dir != "" {
		m, file, found := c.read(r.Dir)
		switch {
		case !found:
			c.report(c.in.ConfigFile, "%s.dir: %s has no module.json — the core worker will ask the module for its manifest at runtime", key, r.Dir)
		case m != nil && m.ID != r.ID:
			c.report(c.in.ConfigFile, "%s.id is %q, but %s says %q", key, r.ID, file, m.ID)
		case m != nil:
			s.panel, s.entry = m.Panel, entryFor(m, s.entry.Base, false)
		}
	}
	c.entries = append(c.entries, s)
}

// dedupe sorts by id and keeps the first entry of each id in configuration
// order (workers: before modules:).
func (c *collector) dedupe() []sourced {
	sort.SliceStable(c.entries, func(i, j int) bool { return c.entries[i].entry.ID < c.entries[j].entry.ID })
	var kept []sourced
	for _, s := range c.entries {
		if n := len(kept); n > 0 && kept[n-1].entry.ID == s.entry.ID {
			c.report(c.in.ConfigFile, "%s: module id %q is already registered by %s — only the first is kept", s.key, s.entry.ID, kept[n-1].key)
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// checkPanels: two modules whose panels are the same page.
func (c *collector) checkPanels(kept []sourced) {
	claimed := map[string]sourced{}
	for _, s := range kept {
		if s.panel == nil {
			continue
		}
		url := panelURL(s.entry.Base, *s.panel)
		if first, ok := claimed[url]; ok {
			c.report(c.in.ConfigFile, "%s: the panel of %q is %s, which is already the panel of %q (%s)", s.key, s.entry.ID, url, first.entry.ID, first.key)
			continue
		}
		claimed[url] = s
	}
}

// checkBases: two modules deployed outside the build on the same or nested
// base paths would each answer some of the other's requests.
func (c *collector) checkBases(kept []sourced) {
	for i, a := range kept {
		for _, b := range kept[i+1:] {
			nested := within(a.entry.Base, b.entry.Base) || within(b.entry.Base, a.entry.Base)
			if a.entry.Bundled || b.entry.Bundled || !nested {
				continue
			}
			c.report(c.in.ConfigFile, "%s.url %s overlaps %s.url %s — give each module a base path of its own", b.key, b.entry.Base, a.key, a.entry.Base)
		}
	}
}
