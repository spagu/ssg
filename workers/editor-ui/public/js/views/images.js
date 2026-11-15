// Pasting, dropping or picking an image (GO-171): the bytes are checked here
// first (the worker checks again), an image over the limit is offered a
// smaller WebP copy with both sizes shown — never shrunk silently — the
// writer describes it (alt text) before it goes in, and it is stored in the
// browser beside the draft BEFORE its link is written into the text, so a
// reload can never leave a link to an image that is gone.

import { h, render } from "../core/vnode.js";
import { image } from "../models/md-commands.js";
import { IMAGE_TYPES, mediaPathFor, sha256Hex, sniffImage, urlOf } from "../models/media-name.js";
import { fits, formatBytes, scaleFor, sizeAfter, usedImages } from "../models/images.js";
import { openDialog } from "../panel/dialog.js";

const ACCEPT = Object.values(IMAGE_TYPES).join(",");

// addImages: files from a paste or drop, or (null) a file picker.
export async function addImages(app, ed, files) {
  const { t } = app;
  const blocked = ed.mediaProblem();
  if (blocked) return app.notify(t(blocked), "warning");
  if (!files) return pick(app, ed);
  for (const file of files) if (!(await addOne(app, ed, file))) break;
}

function pick(app, ed) {
  const input = render(h("input", { type: "file", accept: ACCEPT, multiple: true, class: "visually-hidden", tabindex: "-1" }));
  input.addEventListener("change", () => {
    const files = [...input.files];
    input.remove();
    if (files.length) void addImages(app, ed, files).catch((e) => app.notify(String(e?.message ?? e), "critical"));
  });
  input.addEventListener("cancel", () => input.remove());
  document.body.appendChild(input);
  input.click();
}

async function addOne(app, ed, file) {
  const { t, fmt } = app;
  const { media, limits } = ed.site;
  let bytes = new Uint8Array(await file.arrayBuffer());
  let ext = sniffImage(bytes);
  if (!ext) return app.notify(t("images.notImage", { name: file.name || "?" }), "critical"), false;
  if (bytes.length > limits.maxFileBytes) {
    const smaller = await offerSmaller(app, file, bytes, limits.maxFileBytes);
    if (!smaller) return false;
    bytes = smaller.bytes;
    ext = sniffImage(bytes);
  }
  const waiting = usedImages(ed.doc.body, ed.images()).map((i) => i.size);
  const why = fits(bytes.length, waiting, limits);
  if (why) return app.notify(t(`images.fits.${why}`, { size: formatBytes(bytes.length, fmt.locale), max: formatBytes(why === "file" ? limits.maxFileBytes : limits.maxChangeBytes, fmt.locale), count: limits.maxFilesPerChange - 1 }), "critical"), false;
  const blob = new Blob([bytes], { type: IMAGE_TYPES[ext] });
  const look = URL.createObjectURL(blob);
  const alt = await askAlt(app, look, file.name);
  URL.revokeObjectURL(look);
  if (alt === null) return false;
  const hash = await sha256Hex(bytes);
  const path = mediaPathFor(media.dir, Date.now(), alt || file.name || "image", hash, ext);
  const rec = { draft: ed.doc.key, path, url: urlOf(path, media), alt, type: IMAGE_TYPES[ext], size: bytes.length, name: file.name || "", blob, savedAt: new Date().toISOString() };
  await app.store.putImage(rec);
  ed.addImage(rec);
  ed.text.insert((text, sel) => image(text, sel, alt, rec.url));
  app.notify(t("images.added", { path }), "ok");
  return true;
}

// askAlt: the alt text, "" for a decorative image, or null (cancelled);
// `hint` is the strings key of the line under the field.
export async function askAlt(app, url, name, hint = "images.altHint") {
  const { t } = app;
  const state = { alt: "", decorative: false };
  const problem = { el: null };
  const check = () => {
    if (state.alt.trim() || state.decorative) return true;
    problem.el.textContent = t("images.altMissing");
    document.getElementById("img-alt")?.focus();
    return false;
  };
  const ok = await openDialog({
    title: t("images.altTitle"),
    content: [
      h("img", { src: url, alt: "", class: "img-check" }),
      h("p", { class: "hint" }, name),
      h("div", { class: "field" }, h("label", { for: "img-alt" }, t("images.alt")), h("input", { id: "img-alt", type: "text", "aria-describedby": "img-alt-hint img-problem", on: { input: (e) => (state.alt = e.target.value) } }), h("p", { id: "img-alt-hint", class: "hint" }, t(hint))),
      h("div", { class: "check" }, h("input", { id: "img-deco", type: "checkbox", on: { change: (e) => (state.decorative = e.target.checked) } }), h("label", { for: "img-deco" }, t("images.decorative"))),
      h("p", { id: "img-problem", class: "form-problem", role: "alert", ref: (el) => (problem.el = el) }),
    ],
    actions: [{ label: t("action.cancel"), value: null }, { label: t("images.insert"), value: "go", variant: "primary", submit: true, check }],
  });
  return ok ? (state.decorative ? "" : state.alt.trim()) : null;
}

// shrink: the image redrawn smaller as WebP (JPEG where the browser cannot
// write WebP), smaller again until it fits — at most four tries.
async function shrink(file, max) {
  const bitmap = await createImageBitmap(file);
  const from = { width: bitmap.width, height: bitmap.height };
  let scale = scaleFor(file.size, max);
  let out = null;
  for (let i = 0; i < 4 && scale; i++) {
    const size = sizeAfter(from.width, from.height, scale);
    const canvas = document.createElement("canvas");
    canvas.width = size.width;
    canvas.height = size.height;
    canvas.getContext("2d").drawImage(bitmap, 0, 0, size.width, size.height);
    const blob = await new Promise((resolve) => canvas.toBlob(resolve, "image/webp", 0.85));
    out = { bytes: new Uint8Array(await blob.arrayBuffer()), ...size, type: blob.type };
    scale = out.bytes.length > max ? scale * (scaleFor(out.bytes.length, max) ?? 1) : null;
  }
  bitmap.close();
  return { from, to: out };
}

async function offerSmaller(app, file, bytes, max) {
  const { t, fmt } = app;
  const b = (n) => formatBytes(n, fmt.locale);
  let got;
  try {
    got = await shrink(new Blob([bytes], { type: file.type }), max);
  } catch {
    app.notify(t("images.cannotShrink", { size: b(bytes.length), max: b(max) }), "critical");
    return null;
  }
  const fitsNow = got.to.bytes.length <= max;
  const ok = await openDialog({
    title: t("images.tooLargeTitle"),
    content: [
      h("p", null, t("images.tooLargeBody", { size: b(bytes.length), max: b(max) })),
      h("dl", { class: "facts" },
        h("dt", null, t("images.before")), h("dd", null, t("images.dims", { size: b(bytes.length), width: got.from.width, height: got.from.height, type: file.type || "?" })),
        h("dt", null, t("images.after")), h("dd", null, t("images.dims", { size: b(got.to.bytes.length), width: got.to.width, height: got.to.height, type: got.to.type }))),
      fitsNow ? null : h("p", { class: "bad" }, t("images.stillTooLarge")),
    ],
    actions: [{ label: t("action.cancel"), value: null }, ...(fitsNow ? [{ label: t("images.useSmaller"), value: "go", variant: "primary", submit: true }] : [])],
  });
  return ok ? got.to : null;
}
