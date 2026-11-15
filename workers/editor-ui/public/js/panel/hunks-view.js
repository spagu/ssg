// A diff the worker computed (GET api/revisions/{path}/diff, GO-172) on
// screen, looking exactly like panel/diff-view.js — the same table, signs
// and words — for a comparison the panel cannot make itself (a revision
// against the current file, both read on the worker). A new module beside
// the copies of designer-ui's generic ones, which are never edited here.
// Words from `t`: diff.added, diff.removed, diff.none, diff.gap, diff.label,
// diff.noNewline.

import { h } from "../core/vnode.js";

const OP = { same: "same", removed: "del", added: "add" };
const SIGN = { add: "+", del: "\u2212", same: " " };

// hunksView(t, {hunks, added, removed, identical}, {label}) → a vnode.
export function hunksView(t, diff, { label = null } = {}) {
  if (diff.identical || !diff.hunks?.length) return h("p", { class: "muted" }, t("diff.none"));
  return h(
    "figure",
    { class: "diff" },
    h("figcaption", null, label ? `${label} \u2014 ` : "", t("diff.label", { added: diff.added, removed: diff.removed })),
    diff.hunks.map((hunk, i) =>
      h(
        "div",
        { class: "diff-hunk" },
        i ? h("p", { class: "diff-gap" }, t("diff.gap")) : null,
        h("table", { class: "diff-table" }, h("tbody", null, hunk.lines.map((l) => {
          const op = OP[l.kind] ?? "same";
          return h(
            "tr",
            { class: `diff-${op}` },
            h("td", { class: "diff-n" }, l.a ?? ""),
            h("td", { class: "diff-n" }, l.b ?? ""),
            h("td", { class: "diff-sign", "aria-hidden": "true" }, SIGN[op]),
            h("td", { class: "diff-text" }, op === "same" ? null : h("span", { class: "visually-hidden" }, t(op === "add" ? "diff.added" : "diff.removed"), " "), l.text || " ", l.noNewline ? h("span", { class: "hint" }, " ", t("diff.noNewline")) : null),
          );
        }))),
      ),
    ),
  );
}
