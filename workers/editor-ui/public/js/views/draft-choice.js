// Opening an article that has a draft in this browser, when the article was
// published again since the draft began (models/article-doc.js openChoice):
// the writer chooses — carry on with my draft (its edits now stand on the
// new version), take the repository's version (the draft is dropped), or
// compare the two first. Nothing is chosen for them.

import { h } from "../core/vnode.js";
import { openDialog } from "../panel/dialog.js";
import { diffView } from "../panel/diff-view.js";
import { fieldChanges } from "../models/fm-form.js";

// draftChoice → "mine" | "repository" (Escape keeps the draft: "mine").
export async function draftChoice(app, draft, article) {
  const { t } = app;
  const actions = [
    { label: t("draft.repository"), value: "repository" },
    { label: t("draft.compare"), value: "compare" },
    { label: t("draft.mine"), value: "mine", variant: "primary", submit: true },
  ];
  const body = [
    h("p", null, t("draft.body", { when: app.fmt.dateTime(draft.savedAt) })),
    h("p", { class: "hint" }, t("draft.hint")),
  ];
  for (;;) {
    const choice = await openDialog({ title: t("draft.title"), content: body, actions, wide: true });
    if (choice !== "compare") return choice ?? "mine";
    await openDialog({ title: t("draft.compareTitle"), content: comparison(app, draft, article), actions: [{ label: t("action.close"), value: null }], wide: true });
  }
}

// comparison: the repository's version against the draft — the form keys
// the draft changed, the frontmatter text if it was typed, and the text.
function comparison(app, draft, article) {
  const { t } = app;
  const keys = Object.keys(fieldChanges(draft.original, draft.inputs));
  return [
    keys.length ? h("p", null, t("draft.keys", { keys: keys.map((k) => t(`form.${k}`)).join(", ") })) : null,
    draft.rawEdited ? diffView(t, article.frontmatterRaw, draft.raw, { label: t("publish.frontmatter") }) : null,
    diffView(t, article.body, draft.body, { label: t("publish.text") }),
  ];
}
