// The media library (GO-174 leftover of GO-171): the images already in the
// site's media folder (GET api/media), newest first, to put one in the
// article at the caret. The alt text is asked first, as for a pasted image
// (images.js askAlt). A writer who may add images also gets "Upload a new
// image" here, so the image button leads to both. Thumbnails load from the
// address the site serves each image at — an image published a moment ago
// shows only after the site's next deploy, so every entry also carries its
// name.

import { h } from "../core/vnode.js";
import { image } from "../models/md-commands.js";
import { libraryLabel, matchesImage } from "../models/images.js";
import { openDialog } from "../panel/dialog.js";
import { errorBlock, slot } from "../panel/ui.js";
import { addImages, askAlt } from "./images.js";

// pickImage: an item of the library, "upload", or null.
async function pickImage(app, canUpload) {
  const { t } = app;
  const state = { q: "", items: [], next: null, pick: null, error: null, loading: true };
  const list = slot("library");
  const load = async (cursor) => {
    state.loading = true;
    try {
      const page = await app.api.get("media", { cursor });
      state.items.push(...page.items);
      state.next = page.nextCursor;
    } catch (e) {
      state.error = e;
    }
    state.loading = false;
    draw();
  };
  const choose = (item) => () => (state.pick = item);
  function draw() {
    if (state.error) return list.set(errorBlock(app, state.error, null));
    const shown = state.items.filter((i) => matchesImage(i, state.q));
    list.set([
      shown.length ? h("ul", { class: "library-grid" }, shown.map((i) =>
        h("li", null, h("button", { type: "submit", class: "library-item", on: { click: choose(i) } },
          h("img", { src: i.url, alt: "", loading: "lazy", width: "120", height: "90" }),
          h("span", { class: "library-name" }, libraryLabel(i.path)))))) : null,
      state.loading ? h("p", { role: "status" }, t("state.loading")) : !shown.length ? h("p", { role: "status" }, t(state.items.length ? "library.noMatch" : "library.empty")) : null,
      state.next && !state.loading ? h("button", { type: "button", class: "btn", on: { click: () => void load(state.next) } }, t("library.more")) : null,
    ]);
  }
  const pending = openDialog({
    title: t("library.title"),
    wide: true,
    content: [
      h("p", { class: "hint" }, t("library.lead")),
      h("div", { class: "field" }, h("label", { for: "library-q" }, t("library.search")),
        h("input", { id: "library-q", type: "search", autocomplete: "off", autofocus: true, on: { input: (e) => ((state.q = e.target.value), draw()) } })),
      list.node,
    ],
    actions: [
      { label: t("action.cancel"), value: null },
      ...(canUpload ? [{ label: t("library.upload"), value: "upload" }] : []),
      // Enter in the search field picks the first image shown.
      { label: t("library.first"), value: "go", variant: "primary", submit: true, check: () => !!(state.pick ??= state.items.find((i) => matchesImage(i, state.q)) ?? null) },
    ],
  });
  draw();
  void load(null);
  const answer = await pending;
  return answer === "upload" ? "upload" : answer ? state.pick : null;
}

// openLibrary: pick an image, describe it, insert it at the caret.
export async function openLibrary(app, ed) {
  const picked = await pickImage(app, !ed.mediaProblem());
  if (picked === "upload") return addImages(app, ed, null);
  if (!picked) return;
  const alt = await askAlt(app, picked.url, libraryLabel(picked.path), "library.altHint");
  if (alt === null) return;
  ed.text.insert((text, sel) => image(text, sel, alt, picked.url));
}
