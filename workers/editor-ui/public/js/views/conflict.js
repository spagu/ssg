// Someone published the same article after it was opened here (409
// conflict, decision 4). Nothing was overwritten and nothing typed here is
// lost: the screen shows how this version differs from the one now in the
// repository, puts both side by side — theirs to read, this one to edit into
// a combination — and offers the ways out: publish the combined text over
// theirs, take theirs and drop this draft, or go back to the editor.

import { h } from "../core/vnode.js";
import { conflictView } from "../panel/conflict-view.js";
import { publishDialog } from "../panel/publish-dialog.js";
import { btn } from "../panel/ui.js";
import { imageFiles, send } from "./publish.js";

export function conflictScreen(app, ed, files) {
  const { t } = app;
  const f = files.find((x) => x.path === ed.doc.path) ?? files[0];
  const merged = { text: f.yours ?? f.theirs ?? "" };
  const publishMine = async () => {
    const images = await imageFiles(app, ed, merged.text);
    const got = await publishDialog(t, {
      files: [{ path: f.path, text: t("conflict.replaces") }, ...images.lines],
      message: t("conflict.message", { title: ed.title() }),
      author: app.me.user?.email ?? "",
    });
    if (got) await send(app, ed, { message: got.message, changes: [{ op: "put", path: f.path, version: f.theirVersion ?? null, text: merged.text }, ...images.changes] });
  };
  const takeTheirs = async () => {
    await ed.published();
    app.notify(t("conflict.tookTheirs"));
    ed.after(false);
  };
  const failed = (e) => app.notify(String(e?.message ?? e), "critical");
  const screen = h("section", { class: "conflict-screen", "aria-labelledby": "conflict-head" },
    h("h2", { id: "conflict-head", tabindex: "-1" }, t("conflict.heading")),
    conflictView(t, { files: [{ path: f.path, mine: f.yours ?? null, theirs: f.theirs ?? null }] }),
    f.theirsUnread ? h("p", { class: "callout warning" }, t(`conflict.unread.${f.theirsUnread}`)) : null,
    h("div", { class: "merge" },
      h("div", { class: "field" }, h("label", { for: "merge-theirs" }, t("conflict.theirs")), h("textarea", { id: "merge-theirs", class: "code", rows: "16", readonly: true, value: f.theirs ?? "" })),
      h("div", { class: "field" }, h("label", { for: "merge-mine" }, t("conflict.mine")), h("textarea", { id: "merge-mine", class: "code", rows: "16", "aria-describedby": "merge-hint", value: merged.text, on: { input: (e) => (merged.text = e.target.value) } }),
        h("p", { id: "merge-hint", class: "hint" }, t("conflict.mergeHint")))),
    h("div", { class: "form-actions" },
      app.caps.publish ? btn(t("conflict.publishMine"), () => void publishMine().catch(failed), { variant: "primary" }) : null,
      btn(t("conflict.takeTheirs"), () => void takeTheirs().catch(failed), { variant: "danger" }),
      btn(t("conflict.back"), () => ed.showEditor())));
  ed.showConflict(screen);
  document.getElementById("conflict-head")?.focus();
}
