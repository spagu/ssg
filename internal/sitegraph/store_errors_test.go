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
