package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	ssgroot "github.com/spagu/ssg"
)

// sharedFS is an embedded-like tree holding the four shared files.
func sharedFS() fstest.MapFS {
	m := fstest.MapFS{}
	for _, n := range sharedModuleFiles {
		m["workers/"+n] = &fstest.MapFile{Data: []byte("content of " + n)}
	}
	return m
}

func TestWriteSharedModuleFilesWritesMissing(t *testing.T) {
	dir := t.TempDir()
	notes, err := writeSharedModuleFiles(sharedFS(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != len(sharedModuleFiles) {
		t.Fatalf("want a note per written file, got %v", notes)
	}
	for _, n := range sharedModuleFiles {
		got, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil || string(got) != "content of "+n {
			t.Fatalf("%s: %q, %v", n, got, err)
		}
	}
	// A second run finds them identical and says nothing.
	notes, err = writeSharedModuleFiles(sharedFS(), dir)
	if err != nil || len(notes) != 0 {
		t.Fatalf("second run: %v, %v", notes, err)
	}
}

func TestWriteSharedModuleFilesKeepsADifferentCopy(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "module-manifest.ts")
	if err := os.WriteFile(mine, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes, err := writeSharedModuleFiles(sharedFS(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(mine); string(got) != "edited" {
		t.Fatalf("an existing copy was overwritten: %q", got)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "differs") {
		t.Fatalf("no note about the differing copy: %v", notes)
	}
}

func TestWriteSharedModuleFilesErrors(t *testing.T) {
	if _, err := writeSharedModuleFiles(fstest.MapFS{}, t.TempDir()); err == nil {
		t.Fatal("a missing source file must be an error")
	}
	// A target path that is a directory cannot be read as a file.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, sharedModuleFiles[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeSharedModuleFiles(sharedFS(), dir); err == nil {
		t.Fatal("an unreadable target must be an error")
	}
	// A missing directory cannot be written into.
	_, err := writeSharedModuleFiles(sharedFS(), filepath.Join(t.TempDir(), "absent"))
	if err == nil || errors.Is(err, os.ErrExist) {
		t.Fatalf("want a write error, got %v", err)
	}
}

// The embedded binary carries every shared file (go:embed all:workers).
func TestSharedModuleFilesAreEmbedded(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSharedModuleFiles(ssgroot.EmbeddedWorkers, dir); err != nil {
		t.Fatal(err)
	}
}

func TestRunNewWorkerWritesSharedFiles(t *testing.T) {
	tmp := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	if code := runNewWorker([]string{"newsletter"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, n := range sharedModuleFiles {
		if _, err := os.Stat(filepath.Join("workers", n)); err != nil {
			t.Fatalf("%s not written: %v", n, err)
		}
	}
}

func TestRunNewWorkerSharedFilesFailure(t *testing.T) {
	tmp := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	// A directory where a shared file should go makes the write fail.
	if err := os.MkdirAll(filepath.Join("workers", sharedModuleFiles[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := runNewWorker([]string{"contact-form"}); code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
}
