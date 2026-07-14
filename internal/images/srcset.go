package images

import (
	"fmt"
	"sort"
	"strings"
)

// srcSetOptions carries the parsed imageSrcSet request.
type srcSetOptions struct {
	Widths       []int
	DefaultWidth int
	Base         request // mode/format/quality/resample/upscale for every variant
}

// SrcSet generates one variant per requested width through the shared cache and
// assembles a browser-ready `srcset` string. Widths are sorted, deduplicated,
// bounded by max_variants and — without upscale — skipped above the source width.
func (p *Processor) SrcSet(source string, opts srcSetOptions) (ImageSet, error) {
	if len(opts.Widths) == 0 {
		return ImageSet{}, fmt.Errorf("imageSrcSet: option \"widths\" requires at least one width")
	}
	widths := normalizeWidths(opts.Widths)
	if len(widths) == 0 {
		return ImageSet{}, fmt.Errorf("imageSrcSet: no valid widths (all were <= 0)")
	}
	if len(widths) > p.cfg.MaxVariants {
		return ImageSet{}, fmt.Errorf("imageSrcSet: %d widths exceed max_variants_per_source (%d)", len(widths), p.cfg.MaxVariants)
	}

	info, err := p.Info(source)
	if err != nil {
		return ImageSet{}, fmt.Errorf("imageSrcSet: %w", err)
	}
	upscale := opts.Base.Upscale || p.cfg.AllowUpscale

	set := ImageSet{}
	var parts []string
	for _, w := range widths {
		if !upscale && w > info.Width {
			continue // skip widths larger than the source
		}
		req := opts.Base
		req.Width = w
		if req.Mode == "" {
			req.Mode = "fit_width"
		}
		res, rerr := p.Resize(source, req)
		if rerr != nil {
			return ImageSet{}, fmt.Errorf("imageSrcSet: width %d: %w", w, rerr)
		}
		set.Images = append(set.Images, res)
		parts = append(parts, fmt.Sprintf("%s %dw", res.URL, res.Width))
		if res.Width >= set.Default.Width && (opts.DefaultWidth == 0 || w == opts.DefaultWidth) {
			set.Default = res
		}
	}
	if len(set.Images) == 0 {
		return ImageSet{}, fmt.Errorf("imageSrcSet: every requested width exceeds the source width %d (set \"upscale\" true to allow)", info.Width)
	}
	if set.Default.URL == "" { // requested defaultWidth was skipped → largest wins
		set.Default = set.Images[len(set.Images)-1]
	}
	set.SrcSet = strings.Join(parts, ", ")
	return set, nil
}

// normalizeWidths sorts ascending, deduplicates and drops non-positive widths.
func normalizeWidths(in []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(in))
	for _, w := range in {
		if w <= 0 || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Ints(out)
	return out
}
