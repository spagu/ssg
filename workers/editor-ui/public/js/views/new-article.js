// "New article": a title and the section it goes in. The worker proposes the
// file's name from the title (GET api/content/new-path — ssg's own slug, "-2"
// when taken); the article starts as a draft in this browser and reaches the
// repository only when it is published.

import { h } from "../core/vnode.js";
import { newDoc } from "../models/article-doc.js";
import { sectionLabel } from "../models/article-list.js";
import { openDialog } from "../panel/dialog.js";
import { errorText } from "../panel/ui.js";

// today: the local date as YYYY-MM-DD.
export const today = (d = new Date()) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

export async function newArticle(app, site) {
  const { t } = app;
  // A blog's posts are what is written most: a "post" section comes first.
  const input = { title: "", section: (site.sections.find((s) => s.type === "post") ?? site.sections[0]).path };
  const problem = { el: null };
  const check = () => {
    if (input.title.trim()) return true;
    problem.el.textContent = t("form.titleMissing");
    document.getElementById("new-title")?.focus();
    return false;
  };
  const ok = await openDialog({
    title: t("new.title"),
    content: [
      h("p", { class: "muted" }, t("new.lead")),
      h("div", { class: "field" }, h("label", { for: "new-title" }, t("form.title")), h("input", { id: "new-title", type: "text", required: true, "aria-describedby": "new-problem", on: { input: (e) => (input.title = e.target.value) } })),
      h("div", { class: "field" }, h("label", { for: "new-section" }, t("new.section")),
        h("select", { id: "new-section", "aria-describedby": "new-section-hint", on: { change: (e) => (input.section = e.target.value) } }, site.sections.map((s) => h("option", { value: s.path, selected: s.path === input.section }, sectionLabel(s.path)))),
        h("p", { id: "new-section-hint", class: "hint" }, t("new.sectionHint"))),
      h("p", { id: "new-problem", class: "form-problem", role: "alert", ref: (el) => (problem.el = el) }),
    ],
    actions: [{ label: t("action.cancel"), value: null }, { label: t("new.go"), value: "go", variant: "primary", submit: true, check }],
  });
  if (!ok) return;
  let place;
  try {
    place = await app.api.get("content/new-path", { section: input.section, title: input.title.trim() });
  } catch (e) {
    app.notify(errorText(app, e), "critical");
    return;
  }
  const key = `new:${crypto.randomUUID()}`;
  const origin = input.section.startsWith("mddb:") ? "mddb" : "git";
  await app.store.putDraft({ ...newDoc({ key, path: place.path, section: input.section, origin, title: input.title.trim(), date: today() }), savedAt: new Date().toISOString() });
  app.go(app.href("new", { key }));
}
