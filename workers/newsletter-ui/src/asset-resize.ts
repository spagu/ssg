// The optional width cap on upload (#446): with the Cloudflare Images binding
// NEWSLETTER_IMAGES, an uploaded image wider than NEWSLETTER_ASSETS_MAX_WIDTH
// (default 1200 px — twice a 600 px mail column, sharp on a high-density
// screen) is stored as a copy of that width; without the binding, or for an
// image that is not wider, the file is stored exactly as uploaded.
//
// API used, as documented at developers.cloudflare.com/images/optimization/binding/:
//   (await env.IMAGES.input(stream).transform({ width }).output({ format, quality })).response()
// The default fit is scale-down (never enlarges); the cap is applied only to
// an image wider than it anyway.
//
// Format: kept. JPEG stays JPEG, PNG stays PNG, WebP stays WebP — a silent
// conversion would change what the author chose (WebP's uneven support in
// mail clients is the author's call, the store accepts it as uploaded).
// Quality 80 for JPEG and WebP. PNG gets no quality: for PNG the parameter
// switches to the palette variant (PNG8), a lossy change of a lossless file.
//
// Metadata: the binding does not expose the `metadata` option; its default
// ("copyright") keeps only the Exif copyright tag. Whatever the output holds
// is what the caller reports, by sniffing the stored bytes.
//
// A transform that throws, or returns something that is not the same type no
// wider than the cap, never loses the upload: the original is stored and the
// answer says why (`resize.error`).

import { sniffImage } from "./asset-image";
import type { ImageInfo } from "./asset-image";
import { intVar } from "./lib";
import type { Env } from "./lib";

export const DEFAULT_ASSET_MAX_WIDTH = 1200;
export const RESIZE_QUALITY = 80;
// A narrower cap would make a mail image blurry on every screen; a wider one
// is no longer a cap for a 600 px column.
const MIN_WIDTH = 320;
const MAX_WIDTH = 4000;

// The part of the Images binding this file calls (the documented chain).
export interface ImagesBinding {
  input(stream: ReadableStream<Uint8Array>): {
    transform(options: { width: number }): {
      output(options: { format: string; quality?: number }): Promise<{ response(): Response }>;
    };
  };
}

export const resizeEnabled = (env: Env): boolean => !!env.NEWSLETTER_IMAGES;

// assetMaxWidth: the cap in pixels, clamped to MIN_WIDTH..MAX_WIDTH.
export const assetMaxWidth = (env: Env): number =>
  Math.min(Math.max(intVar(env.NEWSLETTER_ASSETS_MAX_WIDTH, DEFAULT_ASSET_MAX_WIDTH), MIN_WIDTH), MAX_WIDTH);

// What the upload answer says about the cap (only with the binding).
export interface ResizeReport {
  resized: boolean;
  maxWidth: number;
  original: { width: number | null; height: number | null; bytes: number };
  error: "transform_failed" | "unexpected_output" | null;
}

export interface Capped {
  bytes: Uint8Array;
  info: ImageInfo;
  report: ResizeReport | null; // null without the binding
}

// capWidth returns the bytes to store: a width-capped copy when the binding is
// set and the image is wider than the cap, else the upload unchanged.
export async function capWidth(env: Env, bytes: Uint8Array, info: ImageInfo): Promise<Capped> {
  if (!resizeEnabled(env)) return { bytes, info, report: null };
  const maxWidth = assetMaxWidth(env);
  const report: ResizeReport = { resized: false, maxWidth, original: { width: info.width, height: info.height, bytes: bytes.length }, error: null };
  const unchanged = { bytes, info, report };
  if (info.width === null || info.width <= maxWidth) return unchanged;
  let out: Uint8Array;
  try {
    const images = env.NEWSLETTER_IMAGES as ImagesBinding;
    const output = info.type === "image/png" ? { format: info.type } : { format: info.type, quality: RESIZE_QUALITY };
    const result = await images.input(new Blob([bytes]).stream()).transform({ width: maxWidth }).output(output);
    out = new Uint8Array(await result.response().arrayBuffer());
  } catch {
    report.error = "transform_failed";
    return unchanged;
  }
  const outInfo = sniffImage(out);
  if (!outInfo || outInfo.type !== info.type || outInfo.width === null || outInfo.width > maxWidth) {
    report.error = "unexpected_output";
    return unchanged;
  }
  report.resized = true;
  return { bytes: out, info: outInfo, report };
}
