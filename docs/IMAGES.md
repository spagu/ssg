# Image Processing Helpers

*Since v1.8.3.* Build-time image manipulation callable from **templates and
shortcodes**: resize, fit, fill-with-crop, explicit/anchor/focal crops, visual
filters, format conversion with quality control, EXIF orientation
normalization, responsive `srcset` sets — all behind a **deterministic
content-addressed cache** with atomic publishing.

```gotemplate
{{ $image := imageResize "images/hero.jpg" (dict
  "width" 1200 "height" 630 "mode" "fill" "format" "webp" "quality" 82
) }}
<img src="{{ $image.URL }}" width="{{ $image.Width }}" height="{{ $image.Height }}" alt="">
```

Sources are looked up in order: `assets/` → the static dir → the content source
dir → the theme dir. Paths are canonicalized; `..` traversal, absolute paths and
symlink escapes are rejected. Source files are **never modified**. Variants are
published to `/<output>/processed_images/<base>.<hash>.<ext>` and cached in
`.ssg-cache/images/` — the same request never processes twice, and any change to
the source bytes or options changes the hash.

## Helpers

### `imageInfo path`

Metadata without processing: `.Width .Height .Format .AspectRatio .Orientation
.HasAlpha .Animated .FileSize`. EXIF-rotated JPEGs report their upright
dimensions.

### `imageResize path dict`

| Option | Type | Default | Notes |
|--------|------|---------|-------|
| `width`, `height` | int | 0 | per-mode requirements below |
| `mode` | string | `fit` | `scale` · `fit_width` · `fit_height` · `fit` · `fill` |
| `format` | string | `auto` | `auto` keeps the source format (alpha never silently flattened to JPEG) |
| `quality` | int | 82 | 1–100 (JPEG/WebP) |
| `resample` | string | `lanczos` | `nearest` · `linear` · `catmullrom` · `mitchell` · `lanczos` |
| `upscale` | bool | `false` | growing beyond the source is refused unless set |
| `anchor` | string | `center` | used by `fill` |
| `lossless` | bool | `false` | reserved for encoders that support it |

Unknown keys are rejected (`unknown option "widht"`).

- **scale** — exact dimensions, aspect distortion allowed (needs both).
- **fit_width / fit_height** — one exact dimension, the other calculated.
- **fit** — largest size fitting inside the box, aspect preserved.
- **fill** — resize + crop to exact dimensions (anchor or focal point).

### `imageCrop path dict`

Explicit rectangle (`x`,`y`,`width`,`height`), anchor crop (`anchor`, incl.
compass aliases `north`/`southeast`/…), or focal-point crop (`focusX`,`focusY`
∈ 0..1 — the crop window stays inside the image, centred as close to the focus
as possible). Out-of-bounds rectangles are clamped.

### `imageFilter path filters dict`

```gotemplate
{{ $i := imageFilter "photo.jpg" (slice
  (dict "name" "grayscale")
  (dict "name" "contrast" "amount" 1.1)
  (dict "name" "sharpen" "amount" 0.3)
) (dict "format" "webp" "quality" 82) }}
```

Filters run in declared order: `grayscale` `invert` `sepia` (no amount) and
`brightness` −1..1 · `contrast` 0..2 · `saturation` 0..2 · `gamma` 0.1..5 ·
`blur` 0..100 · `sharpen` 0..10 · `opacity` 0..1.

### `imageProcess path ops`

Ordered pipeline of `resize` / `crop` / `filter` / `encode` dicts (each with
`"op"`); a failing operation is identified by index and no partial output is
ever published.

### `imageSrcSet path dict`

```gotemplate
{{ $set := imageSrcSet "hero.jpg" (dict "widths" (slice 480 768 1200) "format" "webp") }}
<img src="{{ $set.Default.URL }}" srcset="{{ $set.SrcSet }}"
     width="{{ $set.Default.Width }}" height="{{ $set.Default.Height }}" alt="">
```

Widths are sorted and deduplicated; invalid ones dropped; widths above the
source are skipped unless `upscale`; up to 20 variants per source; `Default`
is the largest generated variant unless `defaultWidth` picks another.

### `imagePicture path dict`

Emits a `<picture>` with **format fallback** — one `<source>` per format in
declared order, each with its own responsive `srcset`, and an `<img>` fallback
carrying `width`/`height` (zero CLS). Useful even without AVIF: it makes
WebP-with-JPEG-fallback a one-liner.

```gotemplate
{{ $p := imagePicture "hero.jpg" (dict
     "formats" (slice "avif" "webp" "jpeg")
     "widths"  (slice 480 768 1200 1920)
     "sizes"   "(min-width: 64rem) 60vw, 100vw"
     "alt"     "Our team at work"
     "quality" 78) }}
{{ $p.HTML | safeHTML }}
```

The last encodable format becomes the `<img>` fallback; earlier formats become
`<source>` elements. **A format whose encoder is not installed is skipped with
a warning, not a build failure** — so the same template works on a machine
without `avifenc`/`cwebp` (the AVIF `<source>` simply drops out). `formats`
defaults to `["webp", "jpeg"]`.

Result object: `.Sources` (each `.Format`/`.Type`/`.SrcSet`), `.Fallback` (an
image result for the `<img>`), `.Sizes`, `.Alt`, `.Skipped` (formats dropped for
a missing encoder) and `.HTML` (the ready-to-emit markup; pipe through
`safeHTML`).

## Result object

`.URL` `.StaticPath` `.Width` `.Height` `.OriginalWidth` `.OriginalHeight`
`.Format` `.FileSize` `.CacheKey` — no absolute filesystem paths.

## Formats & policies

- **Output**: `jpg`/`jpeg`, `png`, `webp`, `avif` (`auto` = keep source format).
  WebP encoding uses the **optional `cwebp` tool** and AVIF the **optional
  `avifenc` tool** (from libavif) — same optional-binary approach, no CGO, the
  binary stays static. Requesting a format without its tool is a descriptive
  error for `imageResize`/`imageSrcSet`; `imagePicture` instead **skips that
  format with a warning** so the page still builds. AVIF runs ~20–30% smaller
  than WebP; it is opt-in per call, never the default.
- **EXIF**: orientation is normalized before any geometry; metadata (including
  GPS) is stripped — outputs are re-encoded pixels only.
- **Animated GIFs**: processing errors out (`animated_policy: error`) rather
  than silently flattening.
- **Limits**: max source 80 MP / 20 000 px per side, max output 40 MP, max 20
  srcset variants — descriptive errors, no panics (decompression-bomb guard).

## Cache & GC

Key = `sha256(source bytes + normalized ops JSON + processor version)` → name
`<base>.<hash10>.<ext>`. Repeated and clean builds produce identical filenames;
concurrent identical requests process once.

Garbage collection (GO-057): `--images-gc` (config `images_gc: true`) deletes
cache entries the just-finished build no longer references; `--images-gc-dry`
(`images_gc_dry: true`) only reports the file count and bytes that would be
reclaimed. GC runs after generation and never fails the build — errors are
reported as warnings.
