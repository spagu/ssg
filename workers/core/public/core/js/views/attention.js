// "Needs attention": every module's waiting items as sentences, most serious
// first (core sorts and caps them), each with a link into the module's own
// panel. The texts are the modules' — inserted as text, never as markup —
// and the links are core's same-host joins (null when core refused one).

import { h } from "../vnode.js";
import { SEVERITIES, severityCounts } from "../model.js";

// unanswered: how many modules with a summary did not give one this time —
// then "nothing waits" is only true of the others, and the page says so.
export function attentionSection(app, attention, unanswered = 0) {
  const { t, fmt } = app;
  const items = attention?.items ?? [];
  const total = attention?.total ?? items.length;
  const head = h("h2", { id: "attention-title" }, t("attention.title"));
  if (!items.length) {
    return h(
      "section",
      { class: "attention", "aria-labelledby": "attention-title" },
      head,
      h(
        "div",
        { class: `callout ${unanswered ? "sev-warning" : "ok"} all-clear` },
        h("img", { src: "img/all-clear.svg", alt: "", width: "72", height: "54" }),
        h(
          "div",
          null,
          h("p", { class: "callout-title" }, t(unanswered ? "attention.none.partial" : "attention.none.title", { count: unanswered })),
          h("p", null, t(unanswered ? "attention.none.partialText" : "attention.none.text", { count: unanswered })),
        ),
      ),
    );
  }
  const counts = severityCounts(items);
  return h(
    "section",
    { class: "attention", "aria-labelledby": "attention-title" },
    head,
    h("p", { class: "muted" }, SEVERITIES.filter((s) => counts[s]).map((s) => t(`attention.count.${s}`, { count: counts[s] })).join(" · ")),
    h(
      "ul",
      { class: "warnings" },
      items.map((a) =>
        h(
          "li",
          { class: `callout sev-${a.severity}` },
          h("p", { class: "callout-title" }, h("span", { class: `chip chip-${a.severity}` }, t(`severity.${a.severity}`)), " ", h("span", { class: "from" }, a.title)),
          h("p", { class: "attention-text" }, a.text),
          a.href ? h("a", { href: a.href }, t("attention.open", { module: a.title })) : null,
        ),
      ),
    ),
    total > items.length ? h("p", { class: "note" }, t("attention.more", { count: total - items.length, shown: fmt.number(items.length) })) : null,
    unanswered ? h("p", { class: "note" }, t("attention.none.partialText", { count: unanswered })) : null,
  );
}
