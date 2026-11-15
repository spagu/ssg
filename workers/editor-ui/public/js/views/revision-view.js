// One revision of the article (GO-172): who changed it, when and why, and
// how that version differs from the current one — compared on the worker
// (GET api/revisions/{path}/diff). "Restore this version" is an ordinary
// publication of the old text (decision 6): the generic publish dialog, the
// worker's proposed message, and a conflict if the article moved on.

import { h } from "../core/vnode.js";
import { openDialog } from "../panel/dialog.js";
import { hunksView } from "../panel/hunks-view.js";
import { publishDialog } from "../panel/publish-dialog.js";
import { isDirty } from "../models/article-doc.js";
import { send } from "./publish.js";

const seg = (path) => encodeURIComponent(path);

export async function viewRevision(app, ed, rev) {
  const { t } = app;
  const path = ed.doc.path;
  const [read, diff] = await Promise.all([
    app.api.get(`revisions/${seg(path)}/${rev.commitId}`, { cursor: rev.cursor }),
    app.api.get(`revisions/${seg(path)}/diff`, { from: rev.commitId, cursor: rev.cursor }),
  ]);
  const canRestore = app.caps.publish && !read.identical;
  const choice = await openDialog({
    title: t("rev.dialog", { date: app.fmt.dateTime(read.date) }),
    wide: true,
    content: [
      h("dl", { class: "facts" },
        h("dt", null, t("rev.author")), h("dd", null, read.authorName || read.authorEmail),
        h("dt", null, t("rev.date")), h("dd", null, app.fmt.dateTime(read.date)),
        h("dt", null, t("rev.commit")), h("dd", { class: "mono" }, read.commitId.slice(0, 7))),
      h("pre", { class: "rev-message" }, read.message),
      read.identical ? h("p", { class: "muted" }, t("rev.identical")) : diff.tooLarge ? h("p", { class: "callout warning" }, t("rev.tooLarge")) : hunksView(t, diff, { label: t("rev.diffLabel") }),
      canRestore && isDirty(ed.doc) ? h("p", { class: "callout warning" }, t("rev.replacesDraft")) : null,
    ],
    actions: [{ label: t("action.close"), value: null }, ...(canRestore ? [{ label: t("rev.restore"), value: "restore", variant: "primary", submit: true }] : [])],
  });
  if (choice !== "restore") return;
  const got = await publishDialog(t, {
    files: [{ path, text: t("rev.restoreFile", { date: app.fmt.dateTime(read.date) }) }],
    message: read.restore.message,
    author: app.me.user?.email ?? "",
  });
  if (got) await send(app, ed, { message: got.message, changes: [read.restore.change] });
}
