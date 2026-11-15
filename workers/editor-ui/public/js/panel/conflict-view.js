// The conflict screen: someone published the same file after it was read
// here. Nothing was overwritten and nothing of this person's work is lost;
// the screen says that, shows for each text file how this version differs
// from the one now on the branch, and offers the caller's ways out (take the
// new version and keep my edits on top, or discard mine). Generic — the
// words come from `t` (conflict.title, conflict.body, conflict.diff,
// conflict.commit); no panel-specific import.

import { h } from "../core/vnode.js";
import { diffView } from "./diff-view.js";

// conflictView(t, {head, files: [{path, mine, theirs}], keys: [label],
//   actions: [vnode]}) → vnode. `mine`/`theirs` are texts (null: not a text
//   file, or not loaded); `keys` lists settings both sides changed.
export function conflictView(t, { head = null, files = [], keys = [], actions = [] }) {
  return h(
    "section",
    { class: "callout warning conflict", role: "alert", "aria-labelledby": "conflict-title" },
    h("h2", { id: "conflict-title", class: "callout-title" }, t("conflict.title")),
    h("p", null, t("conflict.body")),
    head ? h("p", { class: "hint" }, t("conflict.commit", { commit: head.slice(0, 7) })) : null,
    keys.length ? h("ul", { class: "plain" }, keys.map((k) => h("li", null, k))) : null,
    files.map((f) =>
      f.mine !== null && f.theirs !== null
        ? diffView(t, f.theirs, f.mine, { label: t("conflict.diff", { path: f.path }) })
        : h("p", null, h("code", null, f.path)),
    ),
    actions.length ? h("div", { class: "form-actions" }, actions) : null,
  );
}
