// Package images implements build-time image processing callable from templates
// and shortcodes (audit/images-processing-feature.md): resize/fit/fill, explicit
// and anchor/focal crops, visual filters, format conversion with quality
// settings, EXIF orientation normalization, responsive srcset sets and a
// deterministic content-addressed cache with atomic publishing. Pure Go for
// JPEG/PNG (stdlib + disintegration/imaging); WebP output uses the optional
// cwebp tool, mirroring the existing --webp pipeline.
package images

import (
	"fmt"
	"strings"

	"github.com/disintegration/imaging"
)

// ImageInfo describes a source image without processing it.
type ImageInfo struct {
	SourcePath  string
	Width       int
	Height      int
	Format      string
	AspectRatio float64
	Orientation int
	HasAlpha    bool
	Animated    bool
	FileSize    int64
}

// ImageResult is the template-facing outcome of a processing request. It never
// exposes absolute filesystem paths beyond the publishable StaticPath.
type ImageResult struct {
	URL            string
	StaticPath     string
	SourcePath     string
	Width          int
	Height         int
	OriginalWidth  int
	OriginalHeight int
	Format         string
	FileSize       int64
	CacheKey       string
}

// ImageSet is the result of imageSrcSet: all generated variants, the default
// image and a ready-to-use srcset attribute value.
type ImageSet struct {
	Images  []ImageResult
	Default ImageResult
	SrcSet  string
}

// resample maps the documented filter names onto imaging's resampling kernels.
var resample = map[string]imaging.ResampleFilter{
	"nearest":    imaging.NearestNeighbor,
	"linear":     imaging.Linear,
	"catmullrom": imaging.CatmullRom,
	"mitchell":   imaging.MitchellNetravali,
	"lanczos":    imaging.Lanczos,
}

// anchors maps documented anchor names (plus compass aliases) onto imaging.
var anchors = map[string]imaging.Anchor{
	"center":       imaging.Center,
	"top":          imaging.Top,
	"bottom":       imaging.Bottom,
	"left":         imaging.Left,
	"right":        imaging.Right,
	"top_left":     imaging.TopLeft,
	"top_right":    imaging.TopRight,
	"bottom_left":  imaging.BottomLeft,
	"bottom_right": imaging.BottomRight,
	"north":        imaging.Top,
	"south":        imaging.Bottom,
	"west":         imaging.Left,
	"east":         imaging.Right,
	"northwest":    imaging.TopLeft,
	"northeast":    imaging.TopRight,
	"southwest":    imaging.BottomLeft,
	"southeast":    imaging.BottomRight,
}

// request is the normalized, validated form of one template call. Its fields
// feed both the processing pipeline and the deterministic cache key.
type request struct {
	Op       string  `json:"op"` // resize | crop | filter chain element | encode
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Mode     string  `json:"mode,omitempty"`
	X        int     `json:"x,omitempty"`
	Y        int     `json:"y,omitempty"`
	HasRect  bool    `json:"hasRect,omitempty"`
	Anchor   string  `json:"anchor,omitempty"`
	FocusX   float64 `json:"focusX,omitempty"`
	FocusY   float64 `json:"focusY,omitempty"`
	HasFocus bool    `json:"hasFocus,omitempty"`
	Name     string  `json:"name,omitempty"`   // filter name
	Amount   float64 `json:"amount,omitempty"` // filter amount
	Format   string  `json:"format,omitempty"`
	Quality  int     `json:"quality,omitempty"`
	Resample string  `json:"resample,omitempty"`
	Upscale  bool    `json:"upscale,omitempty"`
	Lossless bool    `json:"lossless,omitempty"`
}

// optInt reads an integer option accepting int/int64/float64 (template dicts
// deliver numbers in any of these).
func optInt(helper, key string, v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("%s: option %q must be a number, got %T", helper, key, v)
	}
}

// optFloat reads a float option.
func optFloat(helper, key string, v any) (float64, error) {
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	default:
		return 0, fmt.Errorf("%s: option %q must be a number, got %T", helper, key, v)
	}
}

// optString / optBool read typed options with helper-prefixed errors.
func optString(helper, key string, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: option %q must be a string, got %T", helper, key, v)
	}
	return s, nil
}

func optBool(helper, key string, v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s: option %q must be a boolean, got %T", helper, key, v)
	}
	return b, nil
}

// parseCommonOption fills shared keys (format/quality/resample/upscale/lossless/
// anchor/focus); returns false when the key is not a common option.
func (r *request) parseCommonOption(helper, key string, v any) (bool, error) {
	var err error
	switch key {
	case "format":
		r.Format, err = optString(helper, key, v)
	case "quality":
		r.Quality, err = optInt(helper, key, v)
	case "resample":
		r.Resample, err = optString(helper, key, v)
	case "upscale":
		r.Upscale, err = optBool(helper, key, v)
	case "lossless":
		r.Lossless, err = optBool(helper, key, v)
	case "anchor":
		r.Anchor, err = optString(helper, key, v)
	case "focusX":
		r.FocusX, err = optFloat(helper, key, v)
		r.HasFocus = true
	case "focusY":
		r.FocusY, err = optFloat(helper, key, v)
		r.HasFocus = true
	default:
		return false, nil
	}
	return true, err
}

// validateCommon checks the shared option surface once per request.
func (r *request) validateCommon(helper string) error {
	if r.Format != "" {
		switch strings.ToLower(r.Format) {
		case "auto", "jpg", "jpeg", "png", "webp":
		default:
			return fmt.Errorf("%s: unsupported output format %q", helper, r.Format)
		}
	}
	if r.Quality < 0 || r.Quality > 100 {
		return fmt.Errorf("%s: quality must be between 1 and 100", helper)
	}
	if r.Resample != "" {
		if _, ok := resample[strings.ToLower(r.Resample)]; !ok {
			return fmt.Errorf("%s: unsupported resample filter %q", helper, r.Resample)
		}
	}
	if r.Anchor != "" {
		if _, ok := anchors[strings.ToLower(r.Anchor)]; !ok {
			return fmt.Errorf("%s: unsupported anchor %q", helper, r.Anchor)
		}
	}
	if r.HasFocus {
		if r.FocusX < 0 || r.FocusX > 1 {
			return fmt.Errorf("%s: focusX must be between 0 and 1", helper)
		}
		if r.FocusY < 0 || r.FocusY > 1 {
			return fmt.Errorf("%s: focusY must be between 0 and 1", helper)
		}
	}
	return nil
}
