// Package deploy publishes a generated site to a hosting provider. It currently
// implements Cloudflare Pages via the Direct Upload API — no external CLI (wrangler)
// or Node.js runtime is required.
package deploy

import (
	"encoding/base64"
	"encoding/hex"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"lukechampine.com/blake3"
)

// specialFiles are Cloudflare Pages control files that are NOT part of the hashed
// asset manifest; they are sent verbatim as separate fields on deployment creation.
var specialFiles = map[string]bool{
	"_headers":     true,
	"_redirects":   true,
	"_routes.json": true,
	"_worker.js":   true,
}

// asset is one uploadable file: its server path ("/css/app.css"), content hash, the
// raw bytes and the resolved MIME type.
type asset struct {
	Path        string
	Hash        string
	Content     []byte
	ContentType string
}

// siteFiles is the result of scanning the output directory: the hashed assets plus
// the raw text of any Cloudflare control files found at the root.
type siteFiles struct {
	assets  []asset
	special map[string]string // filename → text content (_headers, _redirects, …)
}

// fileHash reproduces the Cloudflare Pages content hash: the first 32 hex characters
// of blake3(base64(content) + extension-without-dot). Matching this exactly is what
// lets check-missing deduplicate against already-uploaded assets.
func fileHash(content []byte, ext string) string {
	b64 := base64.StdEncoding.EncodeToString(content)
	sum := blake3.Sum256([]byte(b64 + ext))
	return hex.EncodeToString(sum[:])[:32]
}

// contentTypeFor resolves a MIME type from the file extension, defaulting to a safe
// binary type when unknown.
func contentTypeFor(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// collectSiteFiles walks dir and partitions its files into hashed assets and the
// Cloudflare control files (which are only valid at the site root).
func collectSiteFiles(dir string) (*siteFiles, error) {
	out := &siteFiles{special: map[string]string{}}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(path) // #nosec G304,G122 -- deploy reads the CLI's own output tree; path from local Walk
		if err != nil {
			return err
		}
		// Root-level control files are handled specially, not hashed/uploaded.
		if !strings.Contains(rel, "/") && specialFiles[rel] {
			out.special[rel] = string(content)
			return nil
		}
		ext := strings.TrimPrefix(filepath.Ext(rel), ".")
		out.assets = append(out.assets, asset{
			Path:        "/" + rel,
			Hash:        fileHash(content, ext),
			Content:     content,
			ContentType: contentTypeFor(rel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// manifest maps each asset's server path to its content hash — the JSON body of the
// deployment-creation request.
func (s *siteFiles) manifest() map[string]string {
	m := make(map[string]string, len(s.assets))
	for _, a := range s.assets {
		m[a.Path] = a.Hash
	}
	return m
}

// uniqueByHash returns each distinct asset once (identical files share a hash and
// only need uploading a single time).
func (s *siteFiles) uniqueByHash() []asset {
	seen := make(map[string]bool, len(s.assets))
	uniq := make([]asset, 0, len(s.assets))
	for _, a := range s.assets {
		if seen[a.Hash] {
			continue
		}
		seen[a.Hash] = true
		uniq = append(uniq, a)
	}
	return uniq
}
