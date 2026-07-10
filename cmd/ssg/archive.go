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
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
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
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path) // #nosec G304,G122,G703 -- CLI archives its own output; path from local Walk
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f) // #nosec G110 -- CLI archives its own generated output
		return err
	})
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	return err
}

// createTarGz writes sourceDir to a gzip-compressed tarball at out (v1.8.1).
func createTarGz(sourceDir, out string) error {
	f, err := os.Create(out) // #nosec G304,G703 -- CLI writes its own output archive (name from domain config)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	if err := writeTarball(sourceDir, gz); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

// createTarXz writes sourceDir to an xz-compressed tarball at out (v1.8.1).
func createTarXz(sourceDir, out string) error {
	f, err := os.Create(out) // #nosec G304,G703 -- CLI writes its own output archive (name from domain config)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
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
