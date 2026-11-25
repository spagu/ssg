// The editor's image controls (#446): "Add image…" uploads a file and puts
// it at the cursor of the body; "Image library…" opens the stored images.
// Shown only when the worker has the image store and the account may edit;
// a store that is bound but cannot work says why instead.
//
// `ctx` = {assets (GET /api/settings' assets), body() → the body <textarea>,
// format() → "markdown" | "html"}.

import { h } from "../core/vnode.js";
import { ACCEPTED_TYPES, assetsState, byteSize, checkFile, cleanAlt, uploadNotes } from "../models/assets.js";
import { askAlt, assetErrorText, insertSnippet } from "./asset-insert.js";
import { openLibrary } from "./asset-library.js";
import { btn, errorText } from "./ui.js";

export function imageTools(app, ctx) {
  const { t } = app;
  const state = assetsState(ctx.assets);
  if (state === "off" || !app.can("images")) return null;
  if (state === "problem") {
    return h("div", { class: "callout warning" }, h("p", { class: "callout-title" }, t("asset.problem.title")), h("p", null, t("asset.problem.body", { detail: ctx.assets.problem })));
  }
  const max = byteSize(ctx.assets.maxBytes, app.lang);
  const failed = (e) => app.notify(errorText(app, e), "critical");
  return h(
    "div",
    { class: "image-tools", role: "group", "aria-labelledby": "img-tools-label", "aria-describedby": "img-tools-hint" },
    h("span", { id: "img-tools-label", class: "image-tools-label" }, t("asset.tools")),
    btn(t("asset.add"), () => {
      upload(app, ctx).catch(failed);
    }),
    btn(t("asset.library"), () => {
      openLibrary(app, ctx).catch(failed);
    }),
    h("p", { id: "img-tools-hint", class: "hint" }, t("asset.hint", { max })),
  );
}

// fileField: the file input, checked on change, so a file that cannot be
// sent keeps the dialog open with the reason — said in the field's own
// message and in a live line under it.
function fileField(app, ctx, refs) {
  const { t } = app;
  const check = (el) => {
    const bad = el.files?.[0] ? checkFile(el.files[0], ctx.assets.maxBytes, app.lang) : null;
    const message = bad ? t(bad.key, bad.vars) : "";
    el.setCustomValidity(message);
    refs.message.textContent = message;
  };
  return h(
    "div",
    { class: "field" },
    h("label", { for: "img-file" }, t("asset.file")),
    h("input", { id: "img-file", type: "file", accept: ACCEPTED_TYPES.join(","), required: true, "aria-describedby": "img-file-hint img-file-msg", ref: (el) => (refs.file = el), on: { change: (e) => check(e.currentTarget) } }),
    h("p", { id: "img-file-hint", class: "hint" }, t("asset.file.hint", { max: byteSize(ctx.assets.maxBytes, app.lang) })),
    h("p", { id: "img-file-msg", class: "bad", "aria-live": "polite", ref: (el) => (refs.message = el) }),
  );
}

async function upload(app, ctx) {
  const { t } = app;
  const files = {};
  const answer = await askAlt(app, { title: t("asset.add.title"), body: t("asset.add.body"), confirm: t("asset.add.go"), extra: fileField(app, ctx, files), focus: "img-file" });
  if (!answer) return;
  const file = files.file.files?.[0];
  // The dialog refuses both already; checked again because a dialog's
  // constraint validation is the browser's, and this is cheap.
  const bad = checkFile(file, ctx.assets.maxBytes, app.lang);
  if (bad || !cleanAlt(answer.alt)) return app.notify(bad ? t(bad.key, bad.vars) : t("asset.alt.required"), "critical");
  app.notify(t("asset.uploading"), "info");
  let r;
  try {
    r = await app.api.request("POST", "assets", { raw: file, type: file.type || "application/octet-stream" });
  } catch (e) {
    app.notify(assetErrorText(app, e, ctx.assets), "critical");
    return;
  }
  insertSnippet(ctx.body(), ctx.format(), answer.alt, r.url);
  const { notes, level } = uploadNotes(r, app.lang);
  app.notify(notes.map((n) => t(n.key, n.vars)).join(" "), level);
}
