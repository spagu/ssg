// Image-processing template helpers (audit/images-processing-feature.md):
// imageInfo, imageResize, imageCrop, imageProcess, imageFilter, imageSrcSet.
// Thin adapters over internal/images — the processor owns validation, the
// deterministic cache and atomic publishing. Registered for both theme and
// shortcode templates (shortcodes are a primary use case).
package generator

import (
	"path/filepath"

	"github.com/spagu/ssg/internal/images"
)

// imageProcessor lazily builds the shared processor. Source lookup order per
// the spec: assets/ → static dir → the content source dir → the theme dir.
func (g *Generator) imageProcessor() *images.Processor {
	if g.images != nil {
		return g.images
	}
	staticDir := g.config.StaticDir
	if staticDir == "" {
		staticDir = defaultStaticDir
	}
	// Search order per the spec, plus every extra Markdown root: an image kept
	// beside content in a content_sources directory resolves like one beside
	// the primary source (CONTENT-002).
	dirs := []string{
		"assets",
		staticDir,
		filepath.Join(g.config.ContentDir, g.config.Source),
		filepath.Join(g.config.TemplatesDir, g.config.Template),
	}
	for _, src := range g.config.ContentSources {
		if src.Path != "" {
			dirs = append(dirs, src.Path)
		}
	}
	g.images = images.New(images.Config{
		SourceDirs: dirs,
		OutputDir:  g.config.OutputDir,
		Quiet:      g.config.Quiet,
	})
	return g.images
}

// ImagesGC removes cache entries not referenced by the current build (dry-run
// counts only) and returns files/bytes reclaimed.
func (g *Generator) ImagesGC(dryRun bool) (int, int64, error) {
	return g.imageProcessor().GC(dryRun)
}

func (g *Generator) tmplImageInfo(source string) (images.ImageInfo, error) {
	return g.imageProcessor().Info(source)
}

func (g *Generator) tmplImageResize(source string, opts map[string]any) (images.ImageResult, error) {
	return g.imageProcessor().ResizeDict(source, opts)
}

func (g *Generator) tmplImageCrop(source string, opts map[string]any) (images.ImageResult, error) {
	return g.imageProcessor().CropDict(source, opts)
}

func (g *Generator) tmplImageProcess(source string, ops []any) (images.ImageResult, error) {
	return g.imageProcessor().ProcessList(source, ops)
}

func (g *Generator) tmplImageFilter(source string, filters []any, opts map[string]any) (images.ImageResult, error) {
	return g.imageProcessor().FilterDict(source, filters, opts)
}

func (g *Generator) tmplImageSrcSet(source string, opts map[string]any) (images.ImageSet, error) {
	return g.imageProcessor().SrcSetDict(source, opts)
}

// imageFuncs returns the helper set shared by theme and shortcode templates.
func (g *Generator) imageFuncs() map[string]any {
	return map[string]any{
		"imageInfo":    g.tmplImageInfo,
		"imageResize":  g.tmplImageResize,
		"imageCrop":    g.tmplImageCrop,
		"imageProcess": g.tmplImageProcess,
		"imageFilter":  g.tmplImageFilter,
		"imageSrcSet":  g.tmplImageSrcSet,
	}
}
