// A line diff on screen: the changed runs with a few lines of context, each
// line with its old and new number and a sign (+ added, − removed) that is
// also said in words for screen readers — never colour alone. Generic — the
// words come from `t` (diff.added, diff.removed, diff.none, diff.gap,
// diff.label); no panel-specific import.

import { hunks, lineDiff, stats } from "../core/diff.js";
import { h } from "../core/vnode.js";

const SIGN = { add: "+", del: "\u2212", same: " " };

// diffView(t, before, after, {label}) → a vnode.
export function diffView(t, before, after, { label = null, context = 3 } = {}) {
  const ops = lineDiff(before, after);
  const parts = hunks(ops, context);
  const { added, removed } = stats(ops);
  if (!parts.length) return h("p", { class: "muted" }, t("diff.none"));
  return h(
    "figure",
    { class: "diff" },
    h("figcaption", null, label ? `${label} \u2014 ` : "", t("diff.label", { added, removed })),
    parts.map((part, i) =>
      h(
        "div",
        { class: "diff-hunk" },
        i ? h("p", { class: "diff-gap" }, t("diff.gap")) : null,
        h(
          "table",
          { class: "diff-table" },
          h("tbody", null, part.map((o) =>
            h(
              "tr",
              { class: `diff-${o.op}` },
              h("td", { class: "diff-n" }, o.a ?? ""),
              h("td", { class: "diff-n" }, o.b ?? ""),
              h("td", { class: "diff-sign", "aria-hidden": "true" }, SIGN[o.op]),
              h("td", { class: "diff-text" }, o.op === "same" ? null : h("span", { class: "visually-hidden" }, t(o.op === "add" ? "diff.added" : "diff.removed"), " "), o.text || " "),
            ),
          )),
        ),
      ),
    ),
  );
}
