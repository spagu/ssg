package main

// Packaging a built site: the ZIP and tarball the deploy step hands to a host,
// and the entries each of them must refuse rather than store — a path outside
// the tree, a file that vanished mid-walk, a socket somebody left behind.

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestRunArchivesReportsAFailedTarXz: each format is a separate exit from the
// archive step; the tar.xz one was the last unguarded, and a package that was
// never written must not be reported as a deployment artefact.
func TestRunArchivesReportsAFailedTarXz(t *testing.T) {
	t.Chdir(t.TempDir())
	out := "site-out"
	icovWrite(t, filepath.Join(out, "index.html"), "<html></html>", 0o644)
	// A domain whose archive name lands in a directory that does not exist.
	cfg := &config.Config{Domain: "missing-dir/example.com", OutputDir: out, TarXz: true, Quiet: true}
	err := runArchives(cfg)
	if err == nil || !strings.Contains(err.Error(), "creating tar.xz") {
		t.Fatalf("an unwritable tar.xz must fail the archive step, got %v", err)
	}
}

// TestCreateZipArchiveWarnsPastTheCloudflareLimit: Cloudflare Pages rejects a
// 25 MB upload. Finding that out from Cloudflare instead of from the build is
// the failure this warning exists to prevent.
func TestCreateZipArchiveWarnsPastTheCloudflareLimit(t *testing.T) {
	t.Chdir(t.TempDir())
	out := "site-out"
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	// Incompressible, so the ZIP on disk really is over the limit.
	if err := os.WriteFile(filepath.Join(out, "big.bin"), icovNoise(27<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Domain: "example.com", OutputDir: out}
	var err error
	printed := icovCaptureStdout(t, func() { err = createZipArchive(cfg) })
	if err != nil {
		t.Fatalf("createZipArchive: %v", err)
	}
	if !strings.Contains(printed, "Created deployment package") {
		t.Errorf("size report missing, got %q", printed)
	}
	if !strings.Contains(printed, "25MB limit") {
		t.Errorf("an oversized package must warn about the Pages limit, got %q", printed)
	}
}

// TestZipEntryRefusesAPathOutsideTheTree: an entry that cannot be made relative
// to the archive root has no name to be stored under; storing it anyway would
// write an absolute or escaping path into the ZIP.
func TestZipEntryRefusesAPathOutsideTheTree(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	icovWrite(t, file, "<html></html>", 0o644)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(io.Discard)
	defer func() { _ = zw.Close() }()
	err = zipAddEntry(zw, dir, "elsewhere/index.html", info)
	if err == nil || !strings.Contains(err.Error(), "getting relative path") {
		t.Fatalf("a path outside the tree must be refused, got %v", err)
	}
}

// TestZipEntryReportsAFileThatVanished: a build's own output can disappear
// between the walk and the read (a watcher, a cleaner, a parallel build). The
// archive must fail rather than record an entry with no bytes.
func TestZipEntryReportsAFileThatVanished(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "gone.html")
	icovWrite(t, file, "<html></html>", 0o644)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(io.Discard)
	defer func() { _ = zw.Close() }()
	err = zipAddEntry(zw, dir, file, info)
	if err == nil || !strings.Contains(err.Error(), "opening file") {
		t.Fatalf("a vanished file must fail the entry, got %v", err)
	}
}

// TestZipEntrySurfacesASinkThatStoppedAccepting: once the destination refuses
// bytes, every later entry is lost. The next CreateHeader is where that
// surfaces, and reporting it is what keeps a truncated ZIP from exiting 0.
func TestZipEntrySurfacesASinkThatStoppedAccepting(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(first, icovNoise(64<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "b.html")
	icovWrite(t, second, "<html></html>", 0o644)

	zw := zip.NewWriter(icovFailingWriter{})
	infoFirst, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := zipAddEntry(zw, dir, first, infoFirst); err == nil {
		t.Fatal("a refusing sink must fail the entry being copied")
	}
	infoSecond, err := os.Stat(second)
	if err != nil {
		t.Fatal(err)
	}
	err = zipAddEntry(zw, dir, second, infoSecond)
	if err == nil || !strings.Contains(err.Error(), "creating zip entry") {
		t.Fatalf("the next entry must report the dead sink, got %v", err)
	}
}

// TestTarEntryRefusesAPathOutsideTheTree: as for ZIP — an entry with no
// relative name must not be stored under an absolute one.
func TestTarEntryRefusesAPathOutsideTheTree(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	icovWrite(t, file, "<html></html>", 0o644)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(io.Discard)
	defer func() { _ = tw.Close() }()
	if err := tarAddEntry(tw, dir, "elsewhere/index.html", info); err == nil {
		t.Fatal("a path outside the tree must be refused")
	}
}

// TestTarEntryReportsASymlinkItCannotRead: a symlink is archived by its target,
// read separately from the walk that found it. When that read fails the entry
// has no linkname, and writing it anyway would produce a link to nowhere
// (GO-035 stores links as links, never as bodies).
func TestTarEntryReportsASymlinkItCannotRead(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "latest.html")
	if err := os.Symlink("index.html", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	// The path stops being a symlink before the target is read.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	icovWrite(t, link, "<html></html>", 0o644)

	tw := tar.NewWriter(io.Discard)
	defer func() { _ = tw.Close() }()
	if err := tarAddEntry(tw, dir, link, info); err == nil {
		t.Fatal("an unreadable link target must fail the entry")
	}
}

// TestTarEntryRefusesASocket: tar has no representation for a socket. A build
// that left one in its output must fail the archive with the reason rather than
// panic or write a nonsense header.
func TestTarEntryRefusesASocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "s.sock")
	listener, err := net.Listen("unix", sockPath) // local IPC file, no network
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer func() { _ = listener.Close() }()
	info, err := os.Lstat(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(io.Discard)
	defer func() { _ = tw.Close() }()
	err = tarAddEntry(tw, dir, sockPath, info)
	if err == nil || !strings.Contains(err.Error(), "socket") {
		t.Fatalf("a socket must be refused with a reason, got %v", err)
	}
}

// TestTarEntrySurfacesASinkThatRefuses: the header goes straight to the sink,
// so a destination that stopped accepting bytes is caught before any body is
// copied into a tarball nobody can read.
func TestTarEntrySurfacesASinkThatRefuses(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	icovWrite(t, file, "<html></html>", 0o644)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(icovFailingWriter{})
	if err := tarAddEntry(tw, dir, file, info); err == nil {
		t.Fatal("a refusing sink must fail the header write")
	}
}

// TestTarEntryReportsAFileThatVanished: the tar counterpart of the ZIP case —
// the header is already written, so a failed open must be returned or the
// archive carries an entry with no body.
func TestTarEntryReportsAFileThatVanished(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "gone.html")
	icovWrite(t, file, "<html></html>", 0o644)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer func() { _ = tw.Close() }()
	if err := tarAddEntry(tw, dir, file, info); err == nil {
		t.Fatal("a vanished file must fail the entry")
	}
}

// TestCreateTarXzReportsAFullDestination: the xz stream writes its header the
// moment the writer is made. A destination with no room must fail there — the
// alternative is an empty .tar.xz reported as a deployment package.
func TestCreateTarXzReportsAFullDestination(t *testing.T) {
	probe, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("/dev/full unavailable: %v", err)
	}
	_ = probe.Close()
	dir := t.TempDir()
	icovWrite(t, filepath.Join(dir, "index.html"), "<html></html>", 0o644)
	err = createTarXz(dir, "/dev/full")
	if err == nil {
		t.Fatal("a destination with no space must fail the archive")
	}
	if !strings.Contains(err.Error(), "no space") {
		t.Errorf("error must name the cause, got %v", err)
	}
}
