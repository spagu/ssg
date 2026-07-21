package images

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"os"
	"path/filepath"
	"strings"

	// Register decoders for DecodeConfig/Decode.
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp" // WebP decode support
)

// Info returns normalized metadata for a source image (dimensions read via
// DecodeConfig — no full pixel decode), EXIF-orientation aware: orientations
// 5–8 swap the reported width/height, matching what any processing would yield.
func (p *Processor) Info(source string) (ImageInfo, error) {
	path, err := p.resolve(source)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("imageInfo: %w", err)
	}
	f, err := os.Open(path) // #nosec G304 -- path validated by resolve()
	if err != nil {
		return ImageInfo{}, fmt.Errorf("imageInfo: %w", err)
	}
	defer func() { _ = f.Close() }()

	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("imageInfo: %q is not a supported image: %w", source, err)
	}
	if err := checkDecodable("imageInfo", source, format); err != nil { // SEC-013
		return ImageInfo{}, err
	}
	st, err := f.Stat()
	if err != nil {
		return ImageInfo{}, fmt.Errorf("imageInfo: %w", err)
	}

	info := ImageInfo{
		SourcePath:  filepath.ToSlash(source),
		Width:       cfg.Width,
		Height:      cfg.Height,
		Format:      format,
		Orientation: 1,
		HasAlpha:    modelHasAlpha(cfg.ColorModel),
		FileSize:    st.Size(),
	}
	enrichFormatDetails(f, format, &info)
	if info.Height > 0 {
		info.AspectRatio = float64(info.Width) / float64(info.Height)
	}
	return info, nil
}

// enrichFormatDetails adds format-specific facts: JPEG EXIF orientation (with
// the 90/270° visual dimension swap) and animated-GIF detection.
func enrichFormatDetails(f *os.File, format string, info *ImageInfo) {
	switch format {
	case "jpeg":
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			info.Orientation = exifOrientation(f)
		}
		if info.Orientation >= 5 { // rotated 90/270 → visual dimensions swap
			info.Width, info.Height = info.Height, info.Width
		}
	case "gif":
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			if g, gerr := gif.DecodeAll(f); gerr == nil && len(g.Image) > 1 {
				info.Animated = true
			}
		}
	}
}

// modelHasAlpha reports whether a color model can carry transparency.
func modelHasAlpha(m color.Model) bool {
	switch m {
	case color.RGBAModel, color.NRGBAModel, color.RGBA64Model, color.NRGBA64Model,
		color.AlphaModel, color.Alpha16Model:
		return true
	default:
		return false
	}
}

// exifOrientation extracts the EXIF orientation (1–8) from a JPEG stream,
// returning 1 (upright) when absent or unreadable. Minimal, dependency-free
// parser: JPEG APP1 → "Exif\0\0" → TIFF IFD0 → tag 0x0112.
func exifOrientation(r io.Reader) int {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r, buf); err != nil || buf[0] != 0xFF || buf[1] != 0xD8 {
		return 1 // not a JPEG SOI
	}
	for {
		segment, marker, ok := nextJPEGSegment(r, buf)
		if !ok {
			return 1
		}
		if marker == 0xE1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return orientationFromTIFF(segment[6:])
		}
	}
}

// nextJPEGSegment reads one marker segment; ok=false at scan start/EOI/corrupt
// input (i.e. no EXIF can follow).
func nextJPEGSegment(r io.Reader, buf []byte) (segment []byte, marker byte, ok bool) {
	if _, err := io.ReadFull(r, buf); err != nil || buf[0] != 0xFF {
		return nil, 0, false
	}
	marker = buf[1]
	if marker == 0xDA || marker == 0xD9 { // start of scan / EOI: no EXIF
		return nil, 0, false
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, 0, false
	}
	size := int(binary.BigEndian.Uint16(buf)) - 2
	if size < 0 {
		return nil, 0, false
	}
	segment = make([]byte, size)
	if _, err := io.ReadFull(r, segment); err != nil {
		return nil, 0, false
	}
	return segment, marker, true
}

// orientationFromTIFF scans IFD0 of a TIFF blob for the orientation tag.
func orientationFromTIFF(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(tiff[4:8]))
	if ifd+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[ifd : ifd+2]))
	for i := 0; i < count; i++ {
		entry := ifd + 2 + i*12
		if entry+12 > len(tiff) {
			return 1
		}
		if order.Uint16(tiff[entry:entry+2]) == 0x0112 { // Orientation
			v := int(order.Uint16(tiff[entry+8 : entry+10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// formatFromPath guesses a format string from a file extension (for `auto`).
func formatFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".png":
		return "png"
	case ".webp":
		return "webp"
	case ".gif":
		return "gif"
	default:
		return ""
	}
}
