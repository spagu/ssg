// What adding an image and inserting one from the library share: the alt
// text field (required, checked in the author's language), putting the
// snippet at the cursor of the body, and the asset routes' errors in words.

import { h } from "../core/vnode.js";
import { assetErrorKey, byteSize, cleanAlt, imageSnippet, insertAt } from "../models/assets.js";
import { confirmDialog, errorText } from "./ui.js";

// altField: a required alt text input. Its own message replaces the
// browser's ("Please fill out this field") with one that says what to write,
// and blocks the dialog's submit for whitespace too.
export function altField(app, id, refs) {
  const { t } = app;
  const check = (el) => el.setCustomValidity(cleanAlt(el.value) ? "" : t("asset.alt.required"));
  return h(
    "div",
    { class: "field" },
    h("label", { for: id }, t("asset.alt")),
    h("input", { id, type: "text", required: true, autocomplete: "off", "aria-describedby": `${id}-hint`, ref: (el) => ((refs.alt = el), check(el)), on: { input: (e) => check(e.currentTarget) } }),
    h("p", { id: `${id}-hint`, class: "hint" }, t("asset.alt.hint")),
  );
}

// insertSnippet puts the image at the body's cursor (or in place of the
// selection), moves the cursor after it, and fires `input` so the editor's
// autosave and "Unsaved changes" see it like typing.
export function insertSnippet(body, format, alt, url) {
  const end = body.value.length;
  const { text, cursor } = insertAt(body.value, body.selectionStart ?? end, body.selectionEnd ?? end, imageSnippet(format, alt, url));
  body.value = text;
  body.focus();
  body.setSelectionRange(cursor, cursor);
  body.dispatchEvent(new Event("input", { bubbles: true }));
}

// askAlt asks for the alt text of an image about to be inserted; null when
// the author cancels. `extra` adds fields above it (the file to upload).
export async function askAlt(app, { title, body, confirm, extra = null, focus = "img-alt" }) {
  const refs = {};
  const fields = h("div", { class: "dialog-fields" }, extra, altField(app, "img-alt", refs));
  const ok = await confirmDialog(app, { title, body, confirm, danger: false, extra: fields, focus });
  return ok ? { alt: cleanAlt(refs.alt.value), refs } : null;
}

// assetErrorText: an asset route's error in words; anything else as the
// panel says it everywhere.
export function assetErrorText(app, err, assets) {
  const key = assetErrorKey(err);
  if (!key) return errorText(app, err);
  return app.t(key, { max: byteSize(assets?.maxBytes ?? 0, app.lang), detail: err.detail ?? "" });
}
