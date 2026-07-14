package images

import (
	"fmt"
	"image"
	"image/gif"
	"io"
	"strings"

	"github.com/disintegration/imaging"
)

// validateResize checks an imageResize/resize-op request per documented modes.
func validateResize(helper string, r *request) error {
	if r.Mode == "" {
		r.Mode = "fit"
	}
	if err := r.validateCommon(helper); err != nil {
		return err
	}
	if r.Width < 0 || r.Height < 0 {
		return fmt.Errorf("%s: width and height must be greater than zero", helper)
	}
	switch r.Mode {
	case "scale", "fill":
		if r.Width == 0 || r.Height == 0 {
			return fmt.Errorf("%s: mode %q requires width and height", helper, r.Mode)
		}
	case "fit_width":
		if r.Width == 0 {
			return fmt.Errorf("%s: mode \"fit_width\" requires width", helper)
		}
	case "fit_height":
		if r.Height == 0 {
			return fmt.Errorf("%s: mode \"fit_height\" requires height", helper)
		}
	case "fit":
		if r.Width == 0 && r.Height == 0 {
			return fmt.Errorf("%s: mode \"fit\" requires width and/or height", helper)
		}
	default:
		return fmt.Errorf("%s: unsupported mode %q", helper, r.Mode)
	}
	return nil
}

// validateCrop checks an imageCrop/crop-op request: explicit rect XOR
// anchor/focal crop, with focal coordinates in [0,1].
func validateCrop(helper string, r *request) error {
	if err := r.validateCommon(helper); err != nil {
		return err
	}
	if r.Width <= 0 || r.Height <= 0 {
		return fmt.Errorf("%s: width and height must be greater than zero", helper)
	}
	if r.HasRect && (r.X < 0 || r.Y < 0) {
		return fmt.Errorf("%s: x and y must be greater than or equal to zero", helper)
	}
	return nil
}

// filterRanges documents and enforces the allowed amount per filter.
var filterRanges = map[string][2]float64{
	"brightness": {-1, 1},
	"contrast":   {0, 2},
	"saturation": {0, 2},
	"gamma":      {0.1, 5},
	"blur":       {0, 100},
	"sharpen":    {0, 10},
	"opacity":    {0, 1},
}

// parameterlessFilters need no amount.
var parameterlessFilters = map[string]bool{"grayscale": true, "invert": true, "sepia": true}

// validateFilter checks one element of a filter chain.
func validateFilter(helper string, idx int, r *request) error {
	name := strings.ToLower(r.Name)
	if parameterlessFilters[name] {
		return nil
	}
	rng, ok := filterRanges[name]
	if !ok {
		return fmt.Errorf("%s: filter %d %q: unknown filter", helper, idx, r.Name)
	}
	if r.Amount < rng[0] || r.Amount > rng[1] {
		return fmt.Errorf("%s: filter %d %q: amount must be between %g and %g", helper, idx, r.Name, rng[0], rng[1])
	}
	return nil
}

// validateOp dispatches validation for one imageProcess pipeline element.
func validateOp(helper string, idx int, r *request) error {
	prefix := fmt.Sprintf("%s: operation %d %q", helper, idx, r.Op)
	var err error
	switch r.Op {
	case "resize":
		err = validateResize(helper, r)
	case "crop":
		err = validateCrop(helper, r)
	case "filter":
		err = validateFilter(helper, idx, r)
	case "encode":
		err = r.validateCommon(helper)
	default:
		return fmt.Errorf("%s: unsupported op (want resize, crop, filter or encode)", prefix)
	}
	if err != nil {
		return fmt.Errorf("operation %d: %w", idx, err)
	}
	return nil
}

// applyOp executes one pipeline operation on the in-memory image.
func (p *Processor) applyOp(helper string, idx int, img image.Image, r *request) (image.Image, error) {
	switch r.Op {
	case "resize":
		return p.applyResize(img, r), nil
	case "crop":
		return applyCrop(img, r), nil
	case "filter":
		return applyFilter(img, r), nil
	case "encode":
		return img, nil // consumed by publish()
	default:
		return nil, fmt.Errorf("%s: operation %d %q: unsupported op", helper, idx, r.Op)
	}
}

// applyResize implements the five documented modes. Upscaling is refused
// unless requested (per call or via allow_upscale).
func (p *Processor) applyResize(img image.Image, r *request) image.Image {
	upscale := r.Upscale || p.cfg.AllowUpscale
	kernel := resampleFor(r.Resample)
	switch r.Mode {
	case "scale":
		return resizeScale(img, r.Width, r.Height, upscale, kernel)
	case "fit_width":
		return resizeExact(img, r.Width, 0, img.Bounds().Dx() < r.Width, upscale, kernel)
	case "fit_height":
		return resizeExact(img, 0, r.Height, img.Bounds().Dy() < r.Height, upscale, kernel)
	case "fill":
		return resizeFill(img, r.Width, r.Height, cropAnchor(r), upscale, kernel)
	default:
		return resizeFit(img, r.Width, r.Height, upscale, kernel)
	}
}

// resizeScale forces exact dimensions (aspect distortion allowed).
func resizeScale(img image.Image, w, h int, upscale bool, k imaging.ResampleFilter) image.Image {
	b := img.Bounds()
	if !upscale && (w > b.Dx() || h > b.Dy()) {
		return img
	}
	return imaging.Resize(img, w, h, k)
}

// resizeExact sets one exact dimension (the other calculated); wouldGrow marks
// an upscaling request that must be refused unless allowed.
func resizeExact(img image.Image, w, h int, wouldGrow, upscale bool, k imaging.ResampleFilter) image.Image {
	if wouldGrow && !upscale {
		return img
	}
	return imaging.Resize(img, w, h, k)
}

// resizeFill resizes+crops to exact dimensions; a no-upscale fill larger than
// the source shrinks the box preserving the requested aspect ratio.
func resizeFill(img image.Image, w, h int, anchor imaging.Anchor, upscale bool, k imaging.ResampleFilter) image.Image {
	b := img.Bounds()
	if !upscale && (w > b.Dx() || h > b.Dy()) {
		w, h = clampFill(b.Dx(), b.Dy(), w, h)
	}
	return imaging.Fill(img, w, h, anchor, k)
}

// resizeFit fits inside the box preserving aspect ratio (missing bounds default
// to the source dimensions).
func resizeFit(img image.Image, w, h int, upscale bool, k imaging.ResampleFilter) image.Image {
	b := img.Bounds()
	if w == 0 {
		w = b.Dx()
	}
	if h == 0 {
		h = b.Dy()
	}
	if !upscale && w >= b.Dx() && h >= b.Dy() {
		return img
	}
	return imaging.Fit(img, w, h, k)
}

// clampFill shrinks a fill box that exceeds the source while preserving the
// requested aspect ratio (no-upscale fill).
func clampFill(srcW, srcH, w, h int) (int, int) {
	scaleW := float64(srcW) / float64(w)
	scaleH := float64(srcH) / float64(h)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	if scale >= 1 {
		return w, h
	}
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	return nw, nh
}

// applyCrop implements explicit-rectangle, focal-point and anchor crops.
func applyCrop(img image.Image, r *request) image.Image {
	b := img.Bounds()
	w, h := r.Width, r.Height
	if w > b.Dx() {
		w = b.Dx()
	}
	if h > b.Dy() {
		h = b.Dy()
	}
	switch {
	case r.HasRect:
		rect := image.Rect(r.X, r.Y, r.X+w, r.Y+h).Intersect(b)
		return imaging.Crop(img, rect)
	case r.HasFocus:
		return focalCrop(img, w, h, r.FocusX, r.FocusY)
	default:
		return imaging.CropAnchor(img, w, h, cropAnchor(r))
	}
}

// focalCrop centres a w×h window on the normalized focal point, clamped so the
// crop stays fully inside the image.
func focalCrop(img image.Image, w, h int, fx, fy float64) image.Image {
	b := img.Bounds()
	cx := int(fx*float64(b.Dx())) - w/2
	cy := int(fy*float64(b.Dy())) - h/2
	cx = clampInt(cx, 0, b.Dx()-w)
	cy = clampInt(cy, 0, b.Dy()-h)
	return imaging.Crop(img, image.Rect(b.Min.X+cx, b.Min.Y+cy, b.Min.X+cx+w, b.Min.Y+cy+h))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// cropAnchor resolves the request anchor (default center).
func cropAnchor(r *request) imaging.Anchor {
	if a, ok := anchors[strings.ToLower(r.Anchor)]; ok {
		return a
	}
	return imaging.Center
}

// applyFilter executes one visual filter with a validated amount.
func applyFilter(img image.Image, r *request) image.Image {
	switch strings.ToLower(r.Name) {
	case "grayscale":
		return imaging.Grayscale(img)
	case "invert":
		return imaging.Invert(img)
	case "sepia":
		return imaging.AdjustSaturation(imaging.AdjustContrast(imaging.Grayscale(img), -10), 30)
	case "brightness":
		return imaging.AdjustBrightness(img, r.Amount*100)
	case "contrast":
		return imaging.AdjustContrast(img, (r.Amount-1)*100)
	case "saturation":
		return imaging.AdjustSaturation(img, (r.Amount-1)*100)
	case "gamma":
		return imaging.AdjustGamma(img, r.Amount)
	case "blur":
		return imaging.Blur(img, r.Amount)
	case "sharpen":
		return imaging.Sharpen(img, r.Amount)
	case "opacity":
		return adjustOpacity(img, r.Amount)
	default:
		return img // unreachable after validateFilter
	}
}

// adjustOpacity multiplies the alpha channel by amount ∈ [0,1] (imaging has no
// built-in opacity filter).
func adjustOpacity(img image.Image, amount float64) image.Image {
	out := imaging.Clone(img)
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = uint8(float64(out.Pix[i]) * amount)
	}
	return out
}

// isAnimatedGIF reports whether the GIF stream holds more than one frame.
func isAnimatedGIF(r io.Reader) bool {
	g, err := gif.DecodeAll(r)
	return err == nil && len(g.Image) > 1
}
