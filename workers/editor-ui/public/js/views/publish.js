// Publishing one article (POST api/changes): the generic publish dialog
// lists every file the publication changes with its differences, proposes
// the message ("Update: <title>") and names the author; the worker makes one
// commit of the article and its pasted images (decision 3). Afterwards the
// panel links the commit and says honestly that the site changes only once
// its build has run — the worker does not know when that is. An MDDB
// article shows what the worker answers instead (saved, rebuild started).

import { h } from "../core/vnode.js";
import { changeOf, formProblems, formUsable, messageOf, titleOf } from "../models/article-doc.js";
import { fieldLines } from "../models/fm-form.js";
import { formatBytes, imageChange, usedImages } from "../models/images.js";
import { diffView } from "../panel/diff-view.js";
import { publishDialog } from "../panel/publish-dialog.js";
import { errorText } from "../panel/ui.js";
import { conflictScreen } from "./conflict.js";

// changesDetail: "+3 −1 lines" and, folded, the differences themselves.
function changesDetail(app, doc) {
  const { t } = app;
  if (doc.isNew) return t("publish.newFile");
  const parts = [];
  if (doc.rawEdited) parts.push(diffView(t, doc.originalRaw, doc.raw, { label: t("publish.frontmatter") }));
  if (doc.body !== doc.originalBody) parts.push(diffView(t, doc.originalBody, doc.body, { label: t("publish.text") }));
  if (!parts.length) return t("publish.fieldsOnly");
  return h("details", { class: "pub-diff" }, h("summary", null, t("publish.showChanges")), parts);
}

// imageFiles: the publication's waiting images, for the dialog and the body.
export async function imageFiles(app, ed, body) {
  const imgs = usedImages(body, ed.images());
  const changes = [];
  for (const img of imgs) changes.push(imageChange(img, new Uint8Array(await img.blob.arrayBuffer())));
  const lines = imgs.map((img) => ({ path: img.path, text: app.t("publish.newImage", { size: formatBytes(img.size, app.fmt.locale) }) }));
  return { changes, lines };
}

export async function publishDoc(app, ed) {
  const { t } = app;
  const doc = ed.doc;
  const problems = Object.entries(formProblems(doc));
  if (problems.length) {
    ed.redrawForm();
    document.getElementById(`fm-${problems[0][0]}`)?.focus();
    return app.notify(problems.map(([, id]) => t(id)).join(" "), "critical");
  }
  const change = changeOf(doc);
  if (!change) return app.notify(t("publish.nothing"));
  const images = await imageFiles(app, ed, doc.body);
  if (images.changes.length && !app.caps.media) return app.notify(t("images.noPermission"), "critical");
  const lines = formUsable(doc) && !doc.rawEdited ? fieldLines(doc.isNew ? {} : doc.original, doc.inputs, t) : [];
  const got = await publishDialog(t, {
    lines,
    files: [{ path: doc.path, text: [changesDetail(app, doc), doc.origin === "mddb" ? h("span", { class: "hint" }, " ", t("publish.mddbNoMessage")) : null] }, ...images.lines],
    message: messageOf(doc, t),
    author: app.me.user?.email ?? "",
  });
  if (!got) return;
  await send(app, ed, { message: got.message, changes: [change, ...images.changes] });
}

// send posts a publication and handles every answer.
export async function send(app, ed, body) {
  const { t } = app;
  let r;
  try {
    r = await app.api.post("changes", body);
  } catch (e) {
    if (e.code === "conflict" && Array.isArray(e.body?.files)) return conflictScreen(app, ed, e.body.files);
    if (e.code === "media_needs_repository") ed.blockMedia("images.needsRepository");
    if (e.code === "fields_refused") return app.notify(t("publish.fieldsRefused", { keys: (e.body?.refused ?? []).map((x) => `${x.key} (${x.reason})`).join(", ") }), "critical");
    return app.notify(errorText(app, e), "critical");
  }
  await ed.published();
  app.notify(done(app, r), "ok");
  ed.after(body.changes.some((c) => c.op === "delete"));
}

// done: the success message — the commit linked, and the honest part.
function done(app, r) {
  const { t } = app;
  if (r.origin === "mddb") return [t(r.rebuild?.triggered ? "publish.mddbRebuild" : "publish.mddbSaved"), r.notice ? h("span", { class: "hint" }, " ", t("publish.workerSaid", { notice: r.notice })) : null];
  const commit = String(r.commitId).slice(0, 7);
  const link = /^https:\/\//.test(r.url ?? "") ? h("a", { href: r.url, target: "_blank", rel: "noopener noreferrer" }, t("publish.commitLink", { commit })) : t("publish.commitLink", { commit });
  return [t("publish.done"), " ", link, ". ", t("publish.build")];
}

// deleteDoc removes the article: one publication with a delete.
export async function deleteDoc(app, ed) {
  const { t } = app;
  const doc = ed.doc;
  const got = await publishDialog(t, {
    files: [{ path: doc.path, text: t("publish.deleteFile") }],
    message: t("publish.messageDelete", { title: titleOf(doc) }),
    author: app.me.user?.email ?? "",
    confirm: [{ id: "delete", text: t("publish.deleteConfirm") }],
  });
  if (got) await send(app, ed, { message: got.message, changes: [{ op: "delete", path: doc.path, version: doc.version }] });
}
