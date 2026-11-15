// The editor view: one article — its details form, the Markdown editor with
// its buttons, the preview or history beside it, Publish. Its state is one
// document (models/article-doc.js) kept as a draft in this browser: written
// a moment after each change, at once when the view or the page is left, so
// a reload loses neither a character nor a pasted image. Opening an article
// whose draft is older than the repository's version asks what to keep.

import { h, mount } from "../core/vnode.js";
import { docOf, isDirty, openChoice, rebase, titleOf } from "../models/article-doc.js";
import { newer, takeRescue, writeRescue } from "../store/drafts-db.js";
import { btn, pageHead, slot } from "../panel/ui.js";
import { draftChoice } from "./draft-choice.js";
import { detailsForm } from "./editor-form.js";
import { sidePanel } from "./editor-side.js";
import { textEditor } from "./editor-text.js";
import { showHelp } from "./help.js";
import { addImages } from "./images.js";
import { insertWidget } from "./insert.js";
import { openLibrary } from "./media-library.js";
import { deleteDoc, publishDoc } from "./publish.js";

const SAVE_AFTER_MS = 800;
const read = (app, path) => app.api.get(`content/${encodeURIComponent(path)}`);

// openDoc: the document to edit, after asking about a draft when needed.
async function openDoc(app, route) {
  const { store, storage } = app;
  if (route.name === "new") {
    const key = route.params.key;
    const doc = newer(await store.getDraft(key), takeRescue(storage, key));
    return doc ? { doc } : { gone: true };
  }
  const path = route.params.path;
  const article = await read(app, path);
  const draft = newer(await store.getDraft(path), takeRescue(storage, path));
  const choice = openChoice(draft, article);
  if (choice === "draft") return { doc: { ...draft }, restored: draft.savedAt };
  if (choice === "choose" && (await draftChoice(app, draft, article)) === "mine") return { doc: rebase(draft, article), restored: draft.savedAt };
  if (draft) await store.deleteDraft(path);
  return { doc: docOf(article) };
}

export default async function editor(app, view, route) {
  const { t } = app;
  const site = await app.loadSite();
  const opened = await openDoc(app, route);
  if (opened.gone) {
    mount(view, [pageHead(t("editor.goneTitle")), h("p", null, t("editor.gone")), h("a", { href: app.href("list") }, t("editor.back"))]);
    return null;
  }
  const images = new Map();
  for (const img of await app.store.imagesOf(opened.doc.key)) images.set(img.path, { ...img, local: URL.createObjectURL(img.blob) });
  let unsaved = false;
  let timer = 0;
  let dropped = false; // published or discarded: nothing more to save
  let mediaBlock = site.media ? null : "images.noMediaFolder";
  const status = slot("save-state");
  const main = slot("editor-body");

  const ed = {
    doc: opened.doc,
    site,
    images: () => [...images.values()],
    previewImages: () => new Map([...images.values()].map((i) => [i.url, i.local])),
    title: () => titleOf(ed.doc),
    mediaProblem: () => (!app.caps.media ? "images.noPermission" : mediaBlock),
    blockMedia: (key) => (mediaBlock = key),
    addImage: (rec) => images.set(rec.path, { ...rec, local: URL.createObjectURL(rec.blob) }),
    update(doc, saveNow = false) {
      const textChanged = doc.body !== ed.doc.body;
      ed.doc = doc;
      unsaved = true;
      if (textChanged) ed.changedText?.();
      if (saveNow) return void save().catch(failed);
      if (!timer) timer = setTimeout(() => void save().catch(failed), SAVE_AFTER_MS);
    },
    // published: the draft and its images are done with.
    async published() {
      dropped = true;
      clearTimeout(timer);
      await app.store.deleteDraft(ed.doc.key);
      for (const i of images.values()) URL.revokeObjectURL(i.local);
    },
    after(deleted) {
      if (deleted) return app.go(app.href("list"));
      if (route.name === "new") return app.go(app.href("edit", { path: ed.doc.path }));
      app.refresh();
    },
    showConflict: (node) => main.set(node),
    showEditor: () => void rebaseOnLatest().catch(failed),
  };
  const failed = (e) => app.notify(String(e?.message ?? e), "critical");

  // save writes the draft, or drops it when nothing is left to publish.
  async function save() {
    clearTimeout(timer);
    timer = 0;
    if (!unsaved || dropped) return;
    unsaved = false;
    const at = new Date().toISOString();
    if (ed.doc.isNew || isDirty(ed.doc)) await app.store.putDraft({ ...ed.doc, savedAt: at });
    else await app.store.deleteDraft(ed.doc.key);
    status.set(app.store.persistent ? t("editor.saved", { time: app.fmt.dateTime(at) }) : t("editor.notPersistent"));
  }

  // rebaseOnLatest: back from a conflict — this draft's edits on top of the
  // article as it is now, so publishing again replaces theirs knowingly.
  async function rebaseOnLatest() {
    if (!ed.doc.isNew) ed.update(rebase(ed.doc, await read(app, ed.doc.path)), true);
    draw();
    app.notify(t("conflict.rebased"));
  }

  const onImages = (files) => void addImages(app, ed, files).catch(failed);
  function draw() {
    ed.text = textEditor(app, {
      value: ed.doc.body,
      onChange: (text, saveNow) => ed.update({ ...ed.doc, body: text }, saveNow),
      onImages,
      onHelp: () => void showHelp(app).catch(failed),
      canImage: () => !ed.mediaProblem(),
      onInsert: () => void insertWidget(app, ed).catch(failed),
      onLibrary: () => void openLibrary(app, ed).catch(failed),
      hasLibrary: () => !!site.media,
    });
    main.set([detailsForm(app, ed), h("div", { class: "editor-layout" }, h("section", { class: "editor-main", "aria-label": t("editor.text") }, ed.text.node, status.node), sidePanel(app, ed))]);
  }

  const actions = [
    btn(t("editor.help"), () => void showHelp(app).catch(failed), { variant: "quiet", "aria-keyshortcuts": "Shift+?" }),
    app.caps.delete && !ed.doc.isNew ? btn(t("editor.delete"), () => void deleteDoc(app, ed).catch(failed), { variant: "danger" }) : null,
    app.caps.publish ? btn(t("editor.publish"), () => void save().then(() => publishDoc(app, ed)).catch(failed), { variant: "primary" }) : null,
  ];
  mount(view, [
    h("a", { class: "back", href: app.href("list") }, t("editor.back")),
    pageHead(ed.title(), { lead: t(ed.doc.isNew ? "editor.leadNew" : "editor.lead", { path: ed.doc.path }), actions }),
    app.caps.publish ? null : h("p", { class: "callout" }, t("editor.readOnly")),
    ed.doc.origin === "mddb" ? h("p", { class: "callout" }, t("editor.mddb")) : null,
    app.store.persistent ? null : h("p", { class: "callout warning" }, t("editor.notPersistent")),
    main.node,
  ]);
  draw();
  status.set(opened.restored ? t("editor.restored", { time: app.fmt.dateTime(opened.restored) }) : t("editor.fresh"));
  if (opened.restored) app.notify(t("editor.restored", { time: app.fmt.dateTime(opened.restored) }));
  const onDocKey = (e) => {
    if (e.key === "?" && !/^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) && !document.querySelector("dialog[open]")) void showHelp(app).catch(failed);
  };
  document.addEventListener("keydown", onDocKey);
  return {
    leave: async () => (document.removeEventListener("keydown", onDocKey), save()),
    save,
    unsaved: () => unsaved && !dropped,
    rescue: () => unsaved && !dropped && writeRescue(app.storage, { ...ed.doc, savedAt: new Date().toISOString() }),
  };
}
