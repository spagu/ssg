package voices

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// maxConfigBytes bounds the size of a Piper .onnx.json we are willing to read.
const maxConfigBytes = 4 << 20

// Source describes where voices come from; Load turns it into a Catalog.
type Source struct {
	Root     *os.Root // the voices directory
	Dir      string   // its path, used to build absolute model paths for piper
	Builtins []Voice  // espeak-ng languages
	Piper    bool     // whether the piper binary is installed
	Logger   *slog.Logger
}

// Load discovers Piper models by file name, adds the espeak built-ins, then
// applies voices.yaml. A broken model is skipped with a warning; a broken
// voices.yaml is an error, because the operator asked for something specific.
func (s Source) Load() (Catalog, error) {
	cat := newCatalog()
	for _, v := range s.Builtins {
		cat.add(v)
	}
	if err := s.discoverPiper(cat); err != nil {
		return cat, err
	}
	return cat, s.applyManifest(cat)
}

func (s Source) discoverPiper(cat Catalog) error {
	entries, err := fs.ReadDir(s.Root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read voices dir: %w", err)
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".onnx")
		if !ok || e.IsDir() || !ValidID(id) {
			continue
		}
		if !s.Piper {
			s.Logger.Warn("piper not installed; skipping model", "model", e.Name())
			continue
		}
		v, err := s.piperVoice(id, e.Name(), "")
		if err != nil {
			s.Logger.Warn("skipping piper model", "model", e.Name(), "err", err)
			continue
		}
		cat.add(v)
	}
	return nil
}

// piperVoice builds a Voice from a model and its config, both relative to Dir.
func (s Source) piperVoice(id, model, config string) (Voice, error) {
	if config == "" {
		config = model + ".json"
	}
	if !filepath.IsLocal(model) || !filepath.IsLocal(config) {
		return Voice{}, fmt.Errorf("model and config must stay inside the voices dir")
	}
	info, err := s.Root.Stat(model)
	if err != nil {
		return Voice{}, err
	}
	pc, err := s.readConfig(config)
	if err != nil {
		return Voice{}, err
	}
	return Voice{
		ID: id, Engine: EnginePiper, Lang: pc.Lang(), Name: pc.Dataset,
		Quality: pc.Audio.Quality, SampleRate: pc.Audio.SampleRate,
		Params:    Params{Speed: 1, LengthScale: pc.lengthScale()},
		Model:     filepath.Join(s.Dir, model),
		Config:    filepath.Join(s.Dir, config),
		Revision:  fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()),
		Removable: model == id+".onnx" && config == model+".json",
	}, nil
}

func (s Source) readConfig(name string) (PiperConfig, error) {
	info, err := s.Root.Stat(name)
	if err != nil {
		return PiperConfig{}, err
	}
	if info.Size() > maxConfigBytes {
		return PiperConfig{}, fmt.Errorf("%s: larger than %d bytes", name, maxConfigBytes)
	}
	b, err := s.Root.ReadFile(name)
	if err != nil {
		return PiperConfig{}, err
	}
	return ParsePiperConfig(b)
}

func (s Source) applyManifest(cat Catalog) error {
	b, err := s.Root.ReadFile(ManifestFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	m, err := ParseManifest(b)
	if err != nil {
		return err
	}
	for _, mv := range m.Voices {
		v, err := s.manifestVoice(cat, mv)
		if err != nil {
			return fmt.Errorf("%s: voice %q: %w", ManifestFile, mv.ID, err)
		}
		cat.add(v)
	}
	for lang, id := range m.Defaults {
		if _, ok := cat.Get(id); !ok {
			return fmt.Errorf("%s: default for %q names unknown voice %q", ManifestFile, lang, id)
		}
		cat.defaults[NormalizeLang(lang)] = id
	}
	return nil
}

func (s Source) manifestVoice(cat Catalog, mv ManifestVoice) (Voice, error) {
	base, _ := cat.Get(mv.ID)
	if mv.Engine == EngineEspeak {
		base.EspeakVoice = firstNonEmpty(mv.EspeakVoice, firstNonEmpty(base.EspeakVoice, mv.Lang))
		base.Params.Speed = orOne(base.Params.Speed)
		base.Removable = false
		return mv.apply(base), nil
	}
	if !s.Piper {
		return Voice{}, errors.New("piper is not installed")
	}
	pv, err := s.piperVoice(mv.ID, firstNonEmpty(mv.Model, mv.ID+".onnx"), mv.Config)
	if err != nil {
		return Voice{}, err
	}
	return mv.apply(pv), nil
}

// orOne defaults an unset speed to 1.
func orOne(f float64) float64 {
	if f > 0 {
		return f
	}
	return 1
}
