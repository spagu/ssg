---
title: "Your hero image is your LCP score"
slug: "images-and-lcp"
status: publish
type: post
date: 2026-07-21
tags: [images, performance, core-web-vitals, seo]
excerpt: "Largest Contentful Paint is usually one photograph. A practical tour of SSG's build-time image pipeline — srcset, crops, cache keys — and an honest note about AVIF."
---

Open any page that fails Core Web Vitals and there is a good chance the culprit
is a single JPEG. The Largest Contentful Paint element — the thing Google times
at 2.5 seconds for a passing grade, measured at the 75th percentile of real
visits — is typically a hero image, a large text block, or a video. On a
marketing page it is nearly always the image.

The depressing part is how mechanical the fix is. A
[2025 HTTP Archive analysis](https://krunkit.me/blog/responsive-images-complete-guide)
found that **over 60% of image-heavy sites still serve one image size to every
device**, meaning phones download files three to eight times larger than their
screens can display. That is not a hard engineering problem. It is a build-step
that nobody added.

So let us add it.

## The one-liner that does most of the work

```gotemplate
{{ $set := imageSrcSet "hero.jpg" (dict "widths" (slice 480 768 1200 1920) "format" "webp") }}
<img src="{{ $set.Default.URL }}"
     srcset="{{ $set.SrcSet }}"
     sizes="(min-width: 64rem) 60vw, 100vw"
     width="{{ $set.Default.Width }}"
     height="{{ $set.Default.Height }}"
     fetchpriority="high"
     alt="Waterfall in a granite gorge">
```

Four variants get generated at build time, in WebP, from one source file. What
each attribute is actually doing:

**`srcset`** hands the browser a menu. **`sizes`** tells it how wide the image
will be *rendered* at each breakpoint — without that, the browser assumes the
full viewport width and quietly picks the largest candidate, which undoes the
whole exercise. If your layout is genuinely full-bleed everywhere, SSG's
`image_sizes_attr` config (default `100vw`) covers the generated markup;
anywhere else, write the value yourself, because only you know the layout.

**`width` and `height`** are not decoration. They let the browser reserve the
box before the bytes arrive, which is the difference between a CLS of 0.0 and a
layout that jumps as the photo lands. The result object hands you the real
post-resize numbers, so there is no excuse to guess.

**`fetchpriority="high"`** is the cheapest LCP win available and is worth
setting on exactly one image per page — the hero. Add `<link rel="preload">`
for it if the image is discovered late, for instance from CSS.

Pick 3–5 widths based on file-size steps rather than the device widths of the
month, and stop at 2× DPR. Chasing 3× triples your build output to serve
displays that will downscale the result anyway.

## Crops are a content decision, not a CSS decision

`object-fit: cover` is fine until the interesting part of the photograph is not
in the middle. The pipeline offers three answers, in increasing order of
opinion:

```gotemplate
{{/* fit: largest size inside the box, aspect preserved */}}
{{ $thumb := imageResize "team.jpg" (dict "width" 480 "height" 320 "mode" "fit") }}

{{/* fill: exact dimensions, cropped, anchored where you say */}}
{{ $card := imageResize "team.jpg" (dict "width" 480 "height" 320 "mode" "fill" "anchor" "north") }}

{{/* focal point: crop stays centred on 0..1 coordinates you choose */}}
{{ $face := imageCrop "team.jpg" (dict "width" 400 "height" 400 "focusX" 0.32 "focusY" 0.2) }}
```

`fill` with `"anchor" "north"` is the one that saves group photos, because
faces live at the top and centre-cropping decapitates people. A focal point is
what you want when the subject is somewhere specific and you can be bothered to
say where — frontmatter is a reasonable home for those two numbers.

Filters compose in declared order, which matters more than it sounds:

```gotemplate
{{ $i := imageFilter "photo.jpg" (slice
    (dict "name" "grayscale")
    (dict "name" "contrast" "amount" 1.1)
    (dict "name" "sharpen" "amount" 0.3)
) (dict "format" "webp" "quality" 82) }}
```

Sharpening before downscaling produces crunch; sharpening after produces
detail. The pipeline will not reorder your operations to be helpful.

## Why the filenames look like that

Every generated file is named `<base>.<hash10>.<ext>`, where the hash is
`sha256(source bytes + normalized operations + processor version)`.

This is the boring part that pays rent. The same source with the same
operations produces the same filename on your laptop, in CI, and after you
delete the whole cache — so a CDN can cache it forever, and a rebuild that
changed nothing changes nothing. Edit the photograph and the hash moves, which
is cache invalidation you did not have to think about. Concurrent identical
requests are processed once.

The flip side is that an unused variant lingers in the cache after you stop
referencing it. `--images-gc` deletes what the finished build no longer
mentions; `--images-gc-dry` counts first if you are nervous.

## The AVIF conversation

Here is the part where an honest post beats a feature list.
[AVIF now clears 95% browser support and runs 20–30% smaller than WebP](https://orquitool.com/en/blog/avif-browser-support-2026-compatibility-webp-switch/),
which makes it the obvious default in 2026 — and **SSG does not encode it**.
There is no portable, CGO-free AVIF encoder in Go, and adding a C dependency
would cost the single-static-binary property that makes the tool worth using.

That is a real trade-off, not a rounding error. What to do about it:

- **WebP is not a consolation prize.** It sits at 97%+ support with roughly
  25–34% savings over JPEG and decodes fast. Most of the win, none of the
  toolchain.
- **Let the CDN do it.** Cloudflare Polish, and the equivalent on every other
  image CDN, converts on the edge based on the request's `Accept` header. Your
  build stays pure Go; the last hop serves AVIF to browsers that asked for it.
- **Or hand-roll `<picture>`** for the one image that matters, with an AVIF
  file you produced elsewhere and a WebP fallback from the pipeline. One hero
  is worth the manual step; four hundred thumbnails are not.

Two more constraints worth knowing before they surprise you: WebP encoding
shells out to `cwebp`, so a build box without it gets a descriptive error
rather than a silent JPEG — install `webp` in CI. And animated GIFs error out
instead of being flattened into a still frame, because a silently non-animated
GIF is a bug you find in production.

## What you get for free

EXIF metadata — including GPS coordinates — is stripped, because outputs are
re-encoded pixels rather than copied files. Orientation is normalised first, so
that upright-on-your-phone, sideways-on-the-web classic does not happen.
Sources above 80 megapixels or 20,000 pixels per side are refused, which is a
decompression-bomb guard rather than an opinion about your photography.

The hero on this site's homepage is the whole pipeline in three lines: two WebP
variants at 1920 and 900 pixels, laid under a gradient scrim so the headline
keeps 16.3:1 contrast against a photograph. The logo beside it goes through the
same code but comes out as PNG, because it has an alpha channel and PNG needs
no external encoder.

None of this is clever. It is a build step that runs while you are doing
something else, and it is the difference between a 2.5-second LCP and an
apology to your SEO consultant.

---

**Sources:** [Core Web Vitals 2026 thresholds](https://www.corewebvitals.io/core-web-vitals) ·
[responsive images guide with the HTTP Archive figures](https://krunkit.me/blog/responsive-images-complete-guide) ·
[AVIF browser support in 2026](https://orquitool.com/en/blog/avif-browser-support-2026-compatibility-webp-switch/) ·
[image optimisation for LCP](https://www.sammapix.com/blog/optimize-images-core-web-vitals-2026)
