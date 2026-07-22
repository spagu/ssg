package images

// imagePicture (issue #43): build a <picture> with format fallback — one
// <source> per requested format (avif/webp/jpeg…) in declared order, each with
// its own responsive srcset, plus an <img> fallback carrying width/height so
// CLS stays at zero. A format the machine cannot encode is skipped with a
// warning rather than failing the build, so the same template works on a
// machine without the optional encoder.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PictureSource is one <source> element: a format, its MIME type and srcset.
type PictureSource struct {
	Format string
	Type   string // MIME type, e.g. "image/avif"
	SrcSet string
}

// ImagePicture is the result of imagePicture: the ordered <source> list, the
// <img> fallback, the shared sizes attribute, the formats that were skipped
// because their encoder was missing, and a ready-to-emit HTML string.
type ImagePicture struct {
	Sources  []PictureSource
	Fallback ImageResult
	Sizes    string
	Alt      string
	Skipped  []string
	HTML     string
}

// pictureOptions carries the parsed imagePicture request.
type pictureOptions struct {
	Formats      []string
	Widths       []int
	DefaultWidth int
	Sizes        string
	Alt          string
	Base         request // mode/quality/resample/upscale shared by every variant
}

// formatMIME maps a normalized format to its <source type> MIME value.
func formatMIME(format string) string {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	case "avif":
		return "image/avif"
	default:
		return ""
	}
}

// formatEncodable reports whether this machine can encode the format. JPEG/PNG
// use the standard library; WebP needs cwebp and AVIF needs avifenc on PATH.
func formatEncodable(format string) bool {
	switch strings.ToLower(format) {
	case "webp":
		_, err := exec.LookPath("cwebp")
		return err == nil
	case "avif":
		_, err := exec.LookPath("avifenc")
		return err == nil
	case "jpg", "jpeg", "png", "auto", "":
		return true
	default:
		return false
	}
}

// Picture builds a <picture> across the requested formats. Formats whose encoder
// is absent are skipped (recorded in the result and warned on stderr unless
// Quiet); the last encodable format becomes the <img> fallback.
func (p *Processor) Picture(source string, opts pictureOptions) (ImagePicture, error) {
	formats := opts.Formats
	if len(formats) == 0 {
		formats = []string{"webp", "jpeg"} // WebP with a JPEG fallback: the sensible default
	}
	if len(opts.Widths) == 0 {
		return ImagePicture{}, fmt.Errorf("imagePicture: option \"widths\" requires at least one width")
	}

	var available []string
	var skipped []string
	for _, f := range formats {
		if formatEncodable(f) {
			available = append(available, f)
			continue
		}
		skipped = append(skipped, f)
		if !p.cfg.Quiet {
			fmt.Fprintf(os.Stderr, "   ⚠️  imagePicture: skipping format %q — its encoder is not installed\n", f)
		}
	}
	if len(available) == 0 {
		return ImagePicture{}, fmt.Errorf("imagePicture: none of the requested formats %v can be encoded on this machine", formats)
	}

	pic := ImagePicture{Sizes: opts.Sizes, Alt: opts.Alt, Skipped: skipped}
	// The last available format is the <img> fallback; the rest become <source>s.
	fallbackFormat := available[len(available)-1]
	for _, f := range available {
		set, err := p.pictureSrcSet(source, f, opts)
		if err != nil {
			return ImagePicture{}, err
		}
		if f == fallbackFormat {
			pic.Fallback = set.Default
		}
		if f != fallbackFormat {
			pic.Sources = append(pic.Sources, PictureSource{Format: f, Type: formatMIME(f), SrcSet: set.SrcSet})
		}
	}
	pic.HTML = renderPictureHTML(pic)
	return pic, nil
}

// pictureSrcSet builds the responsive variant set for one format.
func (p *Processor) pictureSrcSet(source, format string, opts pictureOptions) (ImageSet, error) {
	base := opts.Base
	base.Format = format
	set, err := p.SrcSet(source, srcSetOptions{Widths: opts.Widths, DefaultWidth: opts.DefaultWidth, Base: base})
	if err != nil {
		return ImageSet{}, fmt.Errorf("imagePicture: format %q: %w", format, err)
	}
	return set, nil
}

// renderPictureHTML assembles the <picture> markup. The <img> carries
// width/height (zero CLS), the shared srcset and sizes, alt, and lazy/async
// hints that are safe defaults for below-the-fold imagery.
func renderPictureHTML(pic ImagePicture) string {
	var b strings.Builder
	b.WriteString("<picture>")
	for _, s := range pic.Sources {
		b.WriteString("<source type=\"" + s.Type + "\" srcset=\"" + s.SrcSet + "\"")
		if pic.Sizes != "" {
			b.WriteString(" sizes=\"" + pic.Sizes + "\"")
		}
		b.WriteString(">")
	}
	b.WriteString("<img src=\"" + pic.Fallback.URL + "\"")
	if pic.Fallback.Width > 0 && pic.Fallback.Height > 0 {
		fmt.Fprintf(&b, " width=\"%d\" height=\"%d\"", pic.Fallback.Width, pic.Fallback.Height)
	}
	if pic.Sizes != "" {
		b.WriteString(" sizes=\"" + pic.Sizes + "\"")
	}
	b.WriteString(" alt=\"" + htmlAttrEscape(pic.Alt) + "\" loading=\"lazy\" decoding=\"async\"></picture>")
	return b.String()
}

// htmlAttrEscape escapes the minimal set for a double-quoted attribute value.
func htmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}
