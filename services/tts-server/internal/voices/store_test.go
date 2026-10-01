package voices

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	dir, root := voiceDir(t, nil)
	s := &Store{Root: root}
	model, err := s.Stage(strings.NewReader("\x08model"), 100)
	if err != nil || !strings.HasPrefix(model, ".upload-") {
		t.Fatal(err, model)
	}
	cfg, _ := s.Stage(strings.NewReader(goodConfig), 4096)
	if b, err := s.ReadStaged(cfg, 4096); err != nil || string(b) != goodConfig {
		t.Fatal(err)
	}
	if _, err := s.ReadStaged(cfg, 3); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if h, err := s.Head(model, 1); err != nil || h[0] != 0x08 {
		t.Fatal(err)
	}
	if h, err := s.Head(model, 100); err != nil || len(h) != 6 {
		t.Fatal(err, len(h))
	}
	if err := s.Install("bad/id", model, cfg); err == nil {
		t.Fatal("invalid id")
	}
	if err := s.Install("v1", model, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "v1.onnx.json")); err != nil {
		t.Fatal(err)
	}
	m2, _ := s.Stage(strings.NewReader("x"), 10)
	c2, _ := s.Stage(strings.NewReader("x"), 10)
	if err := s.Install("v1", m2, c2); !errors.Is(err, ErrVoiceExists) {
		t.Fatal(err)
	}
	s.Discard(m2, c2, "")
	if err := s.Remove("v1"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"v1", "../x"} {
		if err := s.Remove(id); !errors.Is(err, ErrNoSuchVoice) {
			t.Fatal(id, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestStoreErrors(t *testing.T) {
	dir, root := voiceDir(t, nil)
	s := &Store{Root: root}
	if _, err := s.Stage(strings.NewReader("12345"), 4); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if _, err := s.ReadStaged("missing", 1); err == nil {
		t.Fatal("missing staged file")
	}
	if _, err := s.Head("missing", 1); err == nil {
		t.Fatal("missing head")
	}
	if err := s.Install("v", "missing1", "missing2"); err == nil {
		t.Fatal("rename of missing file")
	}
	// model removable but config is a non-empty directory: second remove fails
	_ = os.WriteFile(filepath.Join(dir, "d.onnx"), nil, 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "d.onnx.json", "x"), 0o700)
	if err := s.Remove("d"); err == nil {
		t.Fatal("want config removal error")
	}
	_ = os.MkdirAll(filepath.Join(dir, "e.onnx", "x"), 0o700)
	if err := s.Remove("e"); err == nil || errors.Is(err, ErrNoSuchVoice) {
		t.Fatal("want model removal error", err)
	}
	_ = root.Close()
	if _, err := s.Stage(strings.NewReader("x"), 4); err == nil {
		t.Fatal("closed root")
	}
}
