package images

import (
	"fmt"
	"image"
	"os"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
)

// processorVersion participates in every cache key so implementation changes
// invalidate previously generated variants.
const processorVersion = "1"

// Config tunes the processor; zero values fall back to the documented defaults.
type Config struct {
	SourceDirs      []string // search order for template-supplied paths
	OutputDir       string   // build output root (published variants land in URLPrefix below it)
	URLPrefix       string   // default "processed_images"
	CacheDir        string   // default ".ssg-cache/images"
	JPEGQuality     int      // default 82
	WebPQuality     int      // default 82
	AllowUpscale    bool
	MaxSourcePixels int // default 80_000_000 (decompression-bomb guard)
	MaxOutputPixels int // default 40_000_000
	MaxDimension    int // default 20_000
	MaxVariants     int // default 20 (srcset widths per source)
	Quiet           bool
}

// Processor executes image requests with a deterministic content-addressed
// cache. Safe for concurrent use; identical concurrent requests are processed
// once (per-key locking).
type Processor struct {
	cfg        Config
	sourceDirs []string
	mu         sync.Mutex
	keyLocks   map[string]*sync.Mutex
	manifest   map[string]bool // cache-relative names referenced by this build
}

// New builds a Processor, applying defaults for unset limits.
func New(cfg Config) *Processor {
	if cfg.URLPrefix == "" {
		cfg.URLPrefix = "processed_images"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = ".ssg-cache/images"
	}
	if cfg.JPEGQuality <= 0 {
		cfg.JPEGQuality = 82
	}
	if cfg.WebPQuality <= 0 {
		cfg.WebPQuality = 82
	}
	if cfg.MaxSourcePixels <= 0 {
		cfg.MaxSourcePixels = 80_000_000
	}
	if cfg.MaxOutputPixels <= 0 {
		cfg.MaxOutputPixels = 40_000_000
	}
	if cfg.MaxDimension <= 0 {
		cfg.MaxDimension = 20_000
	}
	if cfg.MaxVariants <= 0 {
		cfg.MaxVariants = 20
	}
	return &Processor{
		cfg:        cfg,
		sourceDirs: cfg.SourceDirs,
		keyLocks:   map[string]*sync.Mutex{},
		manifest:   map[string]bool{},
	}
}

// Resize handles the imageResize helper: one resize request plus encoding.
func (p *Processor) Resize(source string, req request) (ImageResult, error) {
	req.Op = "resize"
	if err := validateResize("imageResize", &req); err != nil {
		return ImageResult{}, err
	}
	return p.run("imageResize", source, []request{req})
}

// Crop handles the imageCrop helper: explicit rect, anchor or focal crop.
func (p *Processor) Crop(source string, req request) (ImageResult, error) {
	req.Op = "crop"
	if err := validateCrop("imageCrop", &req); err != nil {
		return ImageResult{}, err
	}
	return p.run("imageCrop", source, []request{req})
}

// Filter handles the imageFilter helper: a filter chain plus encoding options.
func (p *Processor) Filter(source string, filters []request, enc request) (ImageResult, error) {
	ops := make([]request, 0, len(filters)+1)
	for i := range filters {
		filters[i].Op = "filter"
		if err := validateFilter("imageFilter", i, &filters[i]); err != nil {
			return ImageResult{}, err
		}
		ops = append(ops, filters[i])
	}
	enc.Op = "encode"
	if err := enc.validateCommon("imageFilter"); err != nil {
		return ImageResult{}, err
	}
	ops = append(ops, enc)
	return p.run("imageFilter", source, ops)
}

// Process handles the imageProcess helper: an ordered operation pipeline.
func (p *Processor) Process(source string, ops []request) (ImageResult, error) {
	for i := range ops {
		if err := validateOp("imageProcess", i, &ops[i]); err != nil {
			return ImageResult{}, err
		}
	}
	return p.run("imageProcess", source, ops)
}

// run resolves the source, consults the cache and — on miss — decodes,
// applies every operation in order and publishes atomically.
func (p *Processor) run(helper, source string, ops []request) (ImageResult, error) {
	path, err := p.resolve(source)
	if err != nil {
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	key, err := p.cacheKey(path, ops)
	if err != nil {
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	unlock := p.lockKey(key)
	defer unlock()

	if res, ok := p.cached(source, path, key, ops); ok {
		return res, nil
	}

	src, info, err := p.decodeSource(helper, path)
	if err != nil {
		return ImageResult{}, err
	}
	img := src
	for i := range ops {
		img, err = p.applyOp(helper, i, img, &ops[i])
		if err != nil {
			return ImageResult{}, err
		}
	}
	if b := img.Bounds(); b.Dx()*b.Dy() > p.cfg.MaxOutputPixels {
		return ImageResult{}, fmt.Errorf("%s: output exceeds max_output_pixels (%d)", helper, p.cfg.MaxOutputPixels)
	}
	return p.publish(helper, source, path, key, ops, img, info)
}

// decodeSource opens+decodes the image, enforcing bomb limits and normalizing
// EXIF orientation before any geometry is calculated. Animated GIFs error per
// the P0 animated_policy (never silently flattened).
func (p *Processor) decodeSource(helper, path string) (image.Image, ImageInfo, error) {
	f, err := os.Open(path) // #nosec G304 -- path validated by resolve()
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("%s: %w", helper, err)
	}
	defer func() { _ = f.Close() }()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("%s: %q is not a supported image: %w", helper, path, err)
	}
	// Format allowlist before the full decode (SEC-013): the header is trusted
	// only as far as naming the format, and unsupported ones never reach the
	// pixel decoders or imaging's transforms.
	if err := checkDecodable(helper, path, format); err != nil {
		return nil, ImageInfo{}, err
	}
	if cfg.Width > p.cfg.MaxDimension || cfg.Height > p.cfg.MaxDimension {
		return nil, ImageInfo{}, fmt.Errorf("%s: source exceeds max_dimension (%d)", helper, p.cfg.MaxDimension)
	}
	if cfg.Width*cfg.Height > p.cfg.MaxSourcePixels {
		return nil, ImageInfo{}, fmt.Errorf("%s: source exceeds max_source_pixels (%d)", helper, p.cfg.MaxSourcePixels)
	}
	if format == "gif" {
		if _, seekErr := f.Seek(0, 0); seekErr == nil {
			if anim := isAnimatedGIF(f); anim {
				return nil, ImageInfo{}, fmt.Errorf("%s: %q is animated; processing would drop animation (animated_policy: error)", helper, path)
			}
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, ImageInfo{}, fmt.Errorf("%s: %w", helper, err)
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("%s: decoding %q: %w", helper, path, err)
	}
	orientation := 1
	if format == "jpeg" {
		if _, err := f.Seek(0, 0); err == nil {
			orientation = exifOrientation(f)
		}
		img = normalizeOrientation(img, orientation)
	}
	b := img.Bounds()
	info := ImageInfo{Width: b.Dx(), Height: b.Dy(), Format: format, Orientation: orientation}
	return img, info, nil
}

// normalizeOrientation bakes the EXIF orientation into pixels so all crops and
// resizes operate on the upright image.
func normalizeOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return imaging.FlipH(img)
	case 3:
		return imaging.Rotate180(img)
	case 4:
		return imaging.FlipV(img)
	case 5:
		return imaging.Transpose(img)
	case 6:
		return imaging.Rotate270(img)
	case 7:
		return imaging.Transverse(img)
	case 8:
		return imaging.Rotate90(img)
	default:
		return img
	}
}

// lockKey serializes work per cache key so identical concurrent requests are
// processed exactly once.
func (p *Processor) lockKey(key string) func() {
	p.mu.Lock()
	l, ok := p.keyLocks[key]
	if !ok {
		l = &sync.Mutex{}
		p.keyLocks[key] = l
	}
	p.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// resampleFor picks the configured kernel, defaulting to Lanczos.
func resampleFor(name string) imaging.ResampleFilter {
	if f, ok := resample[strings.ToLower(name)]; ok {
		return f
	}
	return imaging.Lanczos
}
