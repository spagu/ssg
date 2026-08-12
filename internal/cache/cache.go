// Package cache is the shared engine under every SSG disk cache (GO-091):
// one root (.ssg-cache/<namespace>/), one key formula (sha256 over inputs plus
// an implementation version), atomic writes, and uniform stats/clean/gc
// primitives the `ssg cache` CLI drives.
//
// It deliberately ships PRIMITIVES, not a one-size store: the three existing
// caches (images, external sources, AI) keep their proven on-disk layouts and
// key formulas byte-for-byte — golden-tested — and adopt these building blocks
// underneath. A new cache should compose the same primitives.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultRoot is the shared on-disk root for every SSG cache namespace.
const DefaultRoot = ".ssg-cache"

// Dir returns the directory of a namespace under root ("" = DefaultRoot).
func Dir(root, namespace string) string {
	if root == "" {
		root = DefaultRoot
	}
	return filepath.Join(root, namespace)
}

// Keyer builds a deterministic content-addressed key: an incremental sha256
// over the written parts, finalized with an optional implementation-version
// tag and optional truncation. The images cache's historical formula
// (source bytes + ops JSON + "v"+version, hex[:10]) is expressible exactly.
type Keyer struct {
	h        interface{ Write(p []byte) (int, error) }
	sum      func() []byte
	version  string
	truncate int
}

// NewKeyer returns a Keyer. version "" omits the version tag; truncate 0
// keeps the full hex digest.
func NewKeyer(version string, truncate int) *Keyer {
	h := sha256.New()
	return &Keyer{h: h, sum: func() []byte { return h.Sum(nil) }, version: version, truncate: truncate}
}

// Write adds a part to the key material.
func (k *Keyer) Write(p []byte) { _, _ = k.h.Write(p) }

// WriteString adds a string part.
func (k *Keyer) WriteString(s string) { _, _ = k.h.Write([]byte(s)) }

// WriteDelim adds a part followed by a NUL delimiter (the external-sources
// convention, so "ab"+"c" never collides with "a"+"bc").
func (k *Keyer) WriteDelim(s string) {
	_, _ = k.h.Write([]byte(s))
	_, _ = k.h.Write([]byte{0})
}

// WriteFileContents streams a file's bytes into the key material.
func (k *Keyer) WriteFileContents(path string) error {
	f, err := os.Open(path) // #nosec G304 -- caller-validated cache input path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(k.h.(io.Writer), f)
	return err
}

// Sum finalizes the key: version tag (if any), hex encoding, truncation.
func (k *Keyer) Sum() string {
	if k.version != "" {
		_, _ = k.h.Write([]byte("v" + k.version))
	}
	s := hex.EncodeToString(k.sum())
	if k.truncate > 0 && k.truncate < len(s) {
		return s[:k.truncate]
	}
	return s
}

// WriteAtomic writes an entry into dir under name via a temp file and rename,
// so a partial write is never visible. fill streams the content; perm is the
// final file mode. The temp file keeps the target's extension so tools that
// sniff by suffix (image encoders) behave identically.
func WriteAtomic(dir, name string, perm os.FileMode, fill func(*os.File) error) error {
	// #nosec G301 -- cache directories hold build artifacts
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pattern := "tmp-*"
	if ext := filepath.Ext(name); ext != "" {
		pattern = "tmp-*" + ext
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := fill(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// WriteAtomicBytes is WriteAtomic for a ready byte slice.
func WriteAtomicBytes(dir, name string, perm os.FileMode, data []byte) error {
	return WriteAtomic(dir, name, perm, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

// Stats describes one namespace directory.
type Stats struct {
	Namespace string
	Dir       string
	Entries   int
	Bytes     int64
}

// DirStats counts the files and bytes under a namespace directory. A missing
// directory is zero entries, not an error.
func DirStats(namespace, dir string) (Stats, error) {
	st := Stats{Namespace: namespace, Dir: dir}
	// The walk callback swallows per-entry errors (including a missing root),
	// so a namespace that does not exist yet reports zero entries, no error.
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		st.Entries++
		st.Bytes += info.Size()
		return nil
	})
	return st, err
}

// Clean removes a namespace directory entirely. A missing directory is a no-op.
func Clean(dir string) error {
	if dir == "" {
		return fmt.Errorf("cache clean: empty directory")
	}
	return os.RemoveAll(dir)
}

// GCKeep removes files in dir for which keep returns false (temp files —
// "tmp-" prefix — are always removed). dryRun only counts. Returns files and
// bytes reclaimed. Subdirectories are left alone.
func GCKeep(dir string, keep func(name string) bool, dryRun bool) (files int, bytes int64, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if keep(name) && !strings.HasPrefix(name, "tmp-") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		files++
		bytes += info.Size()
		if !dryRun {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return files, bytes, nil
}

// HumanBytes renders a byte count for the CLI report (KiB/MiB/GiB).
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
