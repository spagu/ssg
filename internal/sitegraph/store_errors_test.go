package sitegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteRefusesUnwritableDir: a target that cannot be written is an error,
// not a silent no-op — for the single-file and the sharded layout alike.
func TestWriteRefusesUnwritableDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no", "such", "dir")
	if _, err := Write(missing, sample(2), 0); err == nil {
		t.Error("single file into a missing dir must fail")
	}
	// Sharded: the shard directory is created, but a FILE sitting where the
	// shard directory should be blocks it.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ShardDir), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, sample(3), 1); err == nil {
		t.Error("sharded write over a file named like the shard dir must fail")
	}
}

// TestWriteLinesFailsOnDirectory: a shard path that is a directory cannot be
// created as a file, and the error surfaces through the sharded writer.
func TestWriteLinesFailsOnDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ShardDir, "pages-1.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, sample(3), 1); err == nil {
		t.Error("a directory in the shard's place must fail the write")
	}
	// The links shard hitting the same wall.
	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ShardDir, "links.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, sample(3), 1); err == nil {
		t.Error("a directory in the links shard's place must fail the write")
	}
}

// TestLoadErrors: every way the artifact can be wrong reads as a distinct,
// named error — missing, unparsable, a schema this build does not speak, or a
// sharded index whose shards are gone or corrupt.
func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != ErrNotFound {
		t.Errorf("missing graph: got %v, want ErrNotFound", err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("{not json")
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), FileName) {
		t.Errorf("unparsable graph: got %v", err)
	}
	write(`{"schema": 99}`)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Errorf("foreign schema: got %v", err)
	}
	write(`{"schema": 1, "sharded": true, "shards": {"pages": ["site-graph/pages-1.jsonl"], "links": "site-graph/links.jsonl"}}`)
	if _, err := Load(dir); err == nil {
		t.Error("index naming a missing page shard must fail")
	}
	if err := os.MkdirAll(filepath.Join(dir, ShardDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ShardDir, "pages-1.jsonl"), []byte("{\"url\":\"/a/\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("index naming a missing links shard must fail")
	}
	if err := os.WriteFile(filepath.Join(dir, ShardDir, "links.jsonl"), []byte("{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "links.jsonl") {
		t.Errorf("corrupt shard line: got %v", err)
	}
	// An unreadable directory in place of the file is a read error that is
	// NOT "not found".
	if err := os.Remove(filepath.Join(dir, FileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, FileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || err == ErrNotFound {
		t.Errorf("a directory where the file should be: got %v", err)
	}
}

// TestWriteLinesReportsEncodeFailure: a record that cannot be encoded fails the
// shard rather than leaving a half-written file that reads as complete.
func TestWriteLinesReportsEncodeFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := writeLines(path, []chan int{make(chan int)}); err == nil {
		t.Error("an unencodable record must fail writeLines")
	}
}

// TestWriteReportsAGraphItCannotEncode: the single-file path encodes before it
// touches the disk, so a graph JSON refuses must fail loudly and leave no
// truncated site-graph.json beside an otherwise healthy build.
func TestWriteReportsAGraphItCannotEncode(t *testing.T) {
	dir := t.TempDir()
	g := sample(2)
	g.Build.Time = unencodableTime()
	files, err := Write(dir, g, 0)
	if err == nil {
		t.Fatalf("an unencodable graph was written: %v", files)
	}
	if files != nil {
		t.Errorf("files = %v with an error; want none", files)
	}
	if _, statErr := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(statErr) {
		t.Errorf("a failed Write left %s behind: %v", FileName, statErr)
	}
}

// TestWriteShardedReportsAnIndexItCannotEncode: the shards are written first,
// so a build stamp that cannot be rendered fails at the LAST step — it must
// still surface as an error, never as a site-graph/ no index names.
func TestWriteShardedReportsAnIndexItCannotEncode(t *testing.T) {
	dir := t.TempDir()
	g := sample(3)
	g.Build.Time = unencodableTime() // lives in the index, not in the shards
	files, err := Write(dir, g, 1)
	if err == nil {
		t.Fatalf("an unencodable index was written: %v", files)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ShardDir, "pages-1.jsonl")); statErr != nil {
		t.Errorf("the page shard should exist before the index fails: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(statErr) {
		t.Errorf("index written despite the encoding error: %v", statErr)
	}
}

// TestWriteLinesReportsAFailedFlush: the records are buffered, so the write
// that actually reaches the disk happens in Flush — a full disk there has to
// fail the shard instead of leaving a short file that still parses as a
// complete one.
func TestWriteLinesReportsAFailedFlush(t *testing.T) {
	const full = "/dev/full" // every write to it fails with ENOSPC
	f, err := os.OpenFile(full, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("no writable %s to stand in for a full disk: %v", full, err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	// One small record: it stays in the buffer, so the failure can only come
	// from the Flush and not from the encoder.
	err = writeLines(full, []Link{{From: "/a/", To: "/b/", Kind: "page"}})
	if err == nil {
		t.Fatal("a shard that never reached the disk reported success")
	}
	if !strings.Contains(err.Error(), full) {
		t.Errorf("flush error = %v; want the underlying write failure", err)
	}
}

// TestLoadRefusesAnIndexItCannotRead: an index whose shard list is the wrong
// shape still passes the schema probe, so Load has to fail on it rather than
// hand back a graph that quietly has no pages.
func TestLoadRefusesAnIndexItCannotRead(t *testing.T) {
	dir := t.TempDir()
	body := `{"schema": 1, "sharded": true, "shards": {"pages": "site-graph/pages-1.jsonl"}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := Load(dir)
	if err == nil {
		t.Fatalf("a shard list that is not a list loaded: %+v", g)
	}
	if g.Schema != 0 || len(g.Pages) != 0 {
		t.Errorf("a failed Load returned a partial graph: %+v", g)
	}
}
