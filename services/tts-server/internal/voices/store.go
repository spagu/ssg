package voices

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
)

// Errors returned by Store.
var (
	ErrTooLarge    = errors.New("upload exceeds size limit")
	ErrVoiceExists = errors.New("voice already exists")
	ErrNoSuchVoice = errors.New("voice not found in voices directory")
)

// Store writes and removes Piper voice files. Every path goes through
// os.Root, so no id or file name can reach outside the voices directory.
type Store struct {
	Root *os.Root
	mu   sync.Mutex
}

// Stage copies r into a hidden temporary file, failing once more than limit
// bytes arrive. The caller must Install or Discard the returned name.
func (s *Store) Stage(r io.Reader, limit int64) (string, error) {
	name := ".upload-" + rand.Text() + ".tmp"
	f, err := s.Root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	err = errors.Join(err, f.Close())
	if err == nil && n > limit {
		err = ErrTooLarge
	}
	if err != nil {
		s.Discard(name)
		return "", err
	}
	return name, nil
}

// Install moves staged files into place as <id>.onnx and <id>.onnx.json.
// The config lands first so discovery never sees a model without one.
func (s *Store) Install(id, modelTmp, configTmp string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid voice id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Root.Stat(id + ".onnx"); err == nil {
		return ErrVoiceExists
	}
	if err := s.Root.Rename(configTmp, id+".onnx.json"); err != nil {
		return err
	}
	return s.Root.Rename(modelTmp, id+".onnx")
}

// Remove deletes a voice's model and config.
func (s *Store) Remove(id string) error {
	if !ValidID(id) {
		return ErrNoSuchVoice
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.Root.Remove(id + ".onnx")
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNoSuchVoice
	}
	if err != nil {
		return err
	}
	if err := s.Root.Remove(id + ".onnx.json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// ReadStaged returns the content of a staged file no larger than limit.
func (s *Store) ReadStaged(name string, limit int64) ([]byte, error) {
	info, err := s.Root.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, ErrTooLarge
	}
	return s.Root.ReadFile(name)
}

// Head returns up to n leading bytes of a staged file.
func (s *Store) Head(name string, n int) ([]byte, error) {
	f, err := s.Root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		err = nil
	}
	return buf[:got], err
}

// Discard removes staged files, ignoring ones already gone.
func (s *Store) Discard(names ...string) {
	for _, n := range names {
		if n != "" {
			_ = s.Root.Remove(n)
		}
	}
}
