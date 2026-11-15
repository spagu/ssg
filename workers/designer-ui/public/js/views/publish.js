// Publishing: one dialog for everything edited — settings and theme files —
// listed in the person's language, with the message, the author, and the
// confirmation a contrast problem needs; then ONE commit through POST
// api/publish (settings and files together, refused whole or written whole).
// A conflict, a refused key or invalid settings are shown in words with the
// way out; nothing edited here is lost.

import { h } from "../core/vnode.js";
import { evaluate, failingWith, pairsOf } from "../models/pairs.js";
import { labelOf } from "../models/labels.js";
import { commitMessage, describe } from "../models/settings-changes.js";
import { describeDraft } from "../models/theme-files.js";
import { findTokensFile, resolve, tokens, valueMap } from "../models/tokens-css.js";
import { openDialog } from "../panel/dialog.js";
import { publishDialog } from "../panel/publish-dialog.js";
import { errorText } from "../panel/ui.js";
import { conflictScreen } from "./publish-conflict.js";

// contrastWarnings: the failing pairs the edits of the tokens file touch.
export function contrastWarnings(app) {
  const theme = app.theme;
  const path = theme?.available ? findTokensFile(theme.files.map((f) => f.path), theme.dir) : null;
  const d = path ? app.themeDrafts.get(path) : null;
  if (!d || d.text === undefined) return [];
  const before = new Map(tokens(d.original).map((x) => [x.name, x]));
  const changed = new Set(tokens(d.text).filter((x) => before.get(x.name)?.light !== x.light || before.get(x.name)?.dark !== x.dark).map((x) => x.name));
  const all = valueMap(d.text);
  const names = [...all.keys()].filter((n) => resolve(all, n, "light") || resolve(all, n, "dark"));
  return failingWith(evaluate(pairsOf(names), all), changed);
}

export async function publishAll(app) {
  const { t } = app;
  if (app.problems.size) return app.notify(t("publish.fixFirst", { count: app.problems.size }), "critical");
  const settings = app.changes.items();
  const files = app.themeDrafts.items();
  if (!settings.length && !files.length) return app.notify(t("publish.nothing"));
  const lines = settings.map((c) => {
    const d = describe(c, t, (e) => labelOf(app, e));
    return { ...d, note: d.advanced && !app.caps.advanced ? t("publish.needsAdvanced") : null };
  });
  const warn = contrastWarnings(app);
  const confirm = warn.length
    ? [{ id: "contrast", text: t("publish.contrastConfirm", { count: warn.length, pairs: warn.slice(0, 4).map((w) => `${w.fg} / ${w.bg} (${t(`scheme.${w.scheme}`)})`).join(", ") }) }]
    : [];
  const names = [...lines.map((l) => l.label), ...files.map((f) => f.path.split("/").pop())];
  const got = await publishDialog(t, {
    lines,
    files: files.map((f) => {
      const d = describeDraft(f);
      return { path: d.path, text: d.image ? t("publish.imageReplaced") : t("publish.lines", { added: d.added, removed: d.removed }) };
    }),
    message: commitMessage(names, t),
    author: app.me.user?.email ?? "",
    confirm,
  });
  if (!got) return;
  await send(app, got.message, settings.length > 0, files.length > 0);
}

// send: the one request. Every refusal leaves the edits as they were.
async function send(app, message, withSettings, withFiles) {
  const { t } = app;
  const body = { message };
  if (withSettings) body.settings = { baseBlobId: app.settings.view.overlayBlobId, changes: app.changes.list() };
  if (withFiles) body.theme = { changes: app.themeDrafts.list() };
  try {
    const r = await app.api.post("publish", body);
    app.themeDrafts.clear();
    if (withSettings) await app.loadSettings({ reload: true, keep: false });
    if (withFiles) await app.loadTheme({ reload: true });
    app.changed();
    app.notify(t("publish.done", { commits: r.commitId.slice(0, 7) }), "ok");
    app.refresh();
  } catch (e) {
    if (e.code === "conflict") await conflictScreen(app, e);
    else if (e.code === "invalid_settings") await problemsDialog(app, t("publish.invalidTitle"), (e.body?.problems ?? []).map(String));
    else if (e.code === "forbidden" && Array.isArray(e.body?.keys)) await problemsDialog(app, t("publish.forbiddenTitle"), [t("publish.forbiddenBody", { permission: e.body.permission }), ...e.body.keys]);
    else app.notify(errorText(app, e), "critical");
  }
}

// problemsDialog lists what the worker refused, one line each.
async function problemsDialog(app, title, items) {
  await openDialog({ title, content: h("ul", { class: "plain" }, items.map((p) => h("li", null, p))), actions: [{ label: app.t("action.close"), value: null }] });
}
