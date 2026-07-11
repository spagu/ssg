package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// writeTarball streams every file under sourceDir into a tar archive written to w
// (v1.8.1). Paths are stored relative to sourceDir with forward slashes.
func writeTarball(sourceDir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	// #nosec G703 -- sourceDir is the CLI's own output directory, not attacker-controlled
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || path == sourceDir {
			return walkErr
		}
		return tarAddEntry(tw, sourceDir, path, info)
	})
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	return err
}

// tarAddEntry writes one file, directory or symlink entry (relative to
// sourceDir) into tw. Symlinks are stored as proper link entries — their target
// goes into the header linkname and no body is copied, since copying the
// target's bytes against a zero-size symlink header aborts the whole archive
// with "write too long" (GO-035).
func tarAddEntry(tw *tar.Writer, sourceDir, path string, info os.FileInfo) error {
	rel, err := filepath.Rel(sourceDir, path)
	if err != nil {
		return err
	}
	link := ""
	if info.Mode()&os.ModeSymlink != 0 {
		if link, err = os.Readlink(path); err != nil {
			return err
		}
	}
	hdr, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	hdr.Name = strings.ReplaceAll(rel, string(os.PathSeparator), "/")
	if info.IsDir() {
		hdr.Name += "/"
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil // directories and symlinks carry no body (GO-035)
	}
	f, err := os.Open(path) // #nosec G304,G122,G703 -- CLI archives its own output; path from local Walk
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(tw, f) // #nosec G110 -- CLI archives its own generated output
	return err
}

// createTarGz writes sourceDir to a gzip-compressed tarball at out (v1.8.1).
// The output file Close is checked via the named return: deferred writes (NFS,
// quota) surface there, and losing that error would report a truncated archive
// as success (GO-035).
func createTarGz(sourceDir, out string) (err error) {
	f, cerr := os.Create(out) // #nosec G304,G703 -- CLI writes its own output archive (name from domain config)
	if cerr != nil {
		return cerr
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	gz := gzip.NewWriter(f)
	if err := writeTarball(sourceDir, gz); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

// createTarXz writes sourceDir to an xz-compressed tarball at out (v1.8.1).
// The output file Close is checked via the named return (GO-035).
func createTarXz(sourceDir, out string) (err error) {
	f, cerr := os.Create(out) // #nosec G304,G703 -- CLI writes its own output archive (name from domain config)
	if cerr != nil {
		return cerr
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	xw, err := xz.NewWriter(f)
	if err != nil {
		return err
	}
	if err := writeTarball(sourceDir, xw); err != nil {
		_ = xw.Close()
		return err
	}
	return xw.Close()
}
