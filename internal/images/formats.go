package images

import "fmt"

// SEC-013: image.Decode dispatches on the file's magic bytes, not its
// extension, and the decoder set is whatever any imported package registered.
// Pulling in github.com/disintegration/imaging also registers the BMP and TIFF
// decoders, so a crafted TIFF renamed "photo.png" would decode and then be run
// through imaging's transforms — the path that panics in CVE-2023-36308
// (imaging <= 1.6.2, no fixed release upstream).
//
// ssg only ever intends to process the four web formats below, so the decoded
// format is checked against them before any transform touches the pixels. That
// removes the residual TIFF/BMP surface entirely instead of relying on the
// vulnerable code merely being unreachable today.

// decodableFormats are the image.Decode format names ssg processes.
var decodableFormats = map[string]bool{
	"jpeg": true,
	"png":  true,
	"gif":  true,
	"webp": true,
}

// Decodable reports whether a format name from image.Decode / image.DecodeConfig
// is one ssg processes.
//
// Exported because the allowlist is the mitigation, not a detail of this
// package: any code that hands a file to imaging needs the same check, and a
// second copy of the list is a second thing to forget to update. The AVIF pass
// gated on the file extension instead and so skipped it entirely (#198).
func Decodable(format string) bool { return decodableFormats[format] }

// checkDecodable rejects a decoded format outside the supported set, naming the
// format so a mislabelled file is obvious from the build log.
func checkDecodable(helper, source, format string) error {
	if decodableFormats[format] {
		return nil
	}
	return fmt.Errorf("%s: %q decodes as %q, which is not a supported image format (supported: jpeg, png, gif, webp)", helper, source, format)
}
