package main

// The reconciliation half of `ssg mddb push-theme` (#190): what is uploaded,
// what is removed, and how a file is described once it is a document.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mddb"
)

// syncTheme upserts every file and removes documents whose file is gone.
func syncTheme(client *mddb.Client, collection, lang string, files []string,
	flags pushThemeFlags, quiet bool) int {
	onDisk := make(map[string]bool, len(files))
	for _, rel := range files {
		onDisk[rel] = true
	}

	validate := flags.validate
	pushed, failed := 0, 0
	for _, rel := range files {
		body, readErr := os.ReadFile(rel) // #nosec G304 -- enumerated from the configured theme dirs
		if readErr != nil {
			errf("⚠️  %s: %v\n", rel, readErr)
			failed++
			continue
		}
		doc := themeDocument(collection, lang, rel, body)
		if validate {
			// A warning is reported, never fatal — MDDB does not fail
			// validation on one either, because a stringified structure is a
			// valid string (#192). An older server has no such endpoint, and
			// saying so once beats saying it per document.
			validate = reportValidation(client, doc, rel, quiet)
		}
		if flags.dry {
			pushed++
			continue
		}
		if addErr := client.Add(doc); addErr != nil {
			errf("⚠️  %s: %v\n", rel, addErr)
			failed++
			continue
		}
		pushed++
	}

	removed := pruneTheme(client, collection, lang, onDisk, flags.dry)
	if !quiet {
		verb := "pushed"
		if flags.dry {
			verb = "would push"
		}
		fmt.Printf("✅ %s %d theme file(s) to %q, removed %d stale document(s)\n",
			verb, pushed, collection, removed)
	}
	if failed > 0 {
		errf("⚠️  %d file(s) failed\n", failed)
		return 1
	}
	return 0
}

// pruneTheme deletes documents whose source file no longer exists. A collection
// that cannot be listed is not fatal: the upserts above already landed, and
// refusing to report them because the cleanup half failed would be worse.
func pruneTheme(client *mddb.Client, collection, lang string, onDisk map[string]bool, dry bool) int {
	docs, err := client.GetAll(collection, lang, 0)
	if err != nil {
		errf("⚠️  could not list %q to remove stale documents: %v\n", collection, err)
		return 0
	}
	removed := 0
	for _, d := range docs {
		if onDisk[d.Key] {
			continue
		}
		if dry {
			removed++
			continue
		}
		if delErr := client.Delete(mddb.DeleteRequest{Collection: collection, Key: d.Key, Lang: lang}); delErr != nil {
			errf("⚠️  removing %s: %v\n", d.Key, delErr)
			continue
		}
		removed++
	}
	return removed
}

// themeDocument describes one file as an MDDB document. The key is the
// project-relative path, so a search hit names the file to open — and the
// checksum makes an unchanged file recognisable without comparing bodies.
func themeDocument(collection, lang, rel string, body []byte) mddb.AddRequest {
	sum := sha256.Sum256(body)
	return mddb.AddRequest{
		Collection: collection,
		Key:        rel,
		Lang:       lang,
		ContentMD:  string(body),
		Meta: map[string][]string{
			"path":     {rel},
			"kind":     {themeKind(rel)},
			"size":     {strconv.Itoa(len(body))},
			"checksum": {hex.EncodeToString(sum[:])},
		},
	}
}

// themeKind labels a file by what it does, so a query can be narrowed to
// stylesheets or to markup.
func themeKind(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".css", ".scss", ".sass":
		return "style"
	case ".js", ".mjs", ".ts":
		return "script"
	case ".html", ".htm", ".tmpl":
		return "template"
	case ".json", ".yaml", ".yml", ".toml":
		return "data"
	}
	return "asset"
}

// themeFiles lists every template and static asset, project-relative and sorted.
//
// It cannot fail: the walk callback swallows per-entry errors — a theme
// directory that is not there is an ordinary project, not a problem — and
// WalkDir only returns what its callback returns. An error return here would be
// a branch no caller could ever take.
func themeFiles(cfg *config.Config) []string {
	var out []string
	for _, base := range []string{cfg.TemplatesDir, cfg.StaticDir} {
		if base == "" {
			continue
		}
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a missing theme directory is simply empty
			}
			out = append(out, filepath.ToSlash(p))
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// reportValidation checks one document before it is stored and prints whatever
// the server has to say about it. It returns false when the server does not
// offer validation, so the caller stops asking.
//
// The point is where the message lands: the render path already recognises a
// Go-stringified structure, but it does so at the next render — another machine,
// possibly weeks later, naming a document whose author has moved on. Here the
// producer is still holding the source.
func reportValidation(client *mddb.Client, doc mddb.AddRequest, rel string, quiet bool) bool {
	result, unsupported, err := client.Validate(doc.Collection, doc.Meta)
	switch {
	case unsupported:
		if !quiet {
			fmt.Printf("   ℹ️  this MDDB has no /v1/validate (2.12.0+); pushing without checking\n")
		}
		return false
	case err != nil:
		// A validation that cannot run must not stop a write: the document is
		// fine as far as anyone knows, and refusing to store it would make a
		// diagnostic into an outage.
		errf("⚠️  %s: could not validate: %v\n", rel, err)
		return true
	}
	for _, w := range result.Warnings {
		errf("⚠️  %s: %s\n", rel, w)
	}
	for _, e := range result.Errors {
		errf("⚠️  %s: %s\n", rel, e)
	}
	return true
}
