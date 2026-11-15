// What happens when someone published in between (409 conflict): nothing was
// overwritten. One screen for the whole publication: the panel reads the new
// settings and theme, keeps this person's edits on top of them, names the
// settings the other publication touched too and shows, for each text file,
// how this version differs from theirs — then keep the edits (to publish
// again) or drop the ones in conflict.

import { conflictView } from "../panel/conflict-view.js";
import { openDialog } from "../panel/dialog.js";
import { labelOf } from "../models/labels.js";
import { sameValue } from "../models/settings-changes.js";

export async function conflictScreen(app, err) {
  const { t } = app;
  const conflicts = Array.isArray(err.body?.conflicts) ? err.body.conflicts : [];
  const overlay = app.settings.view.overlayFile;
  const settingsHit = conflicts.some((c) => c.path === overlay);
  const before = new Map(app.changes.items().map((c) => [c.entry.segments.join("."), c.entry.value]));
  if (settingsHit) await app.loadSettings({ reload: true, keep: true });
  const keys = app.changes.items().filter((c) => settingsHit && before.has(c.entry.segments.join(".")) && !sameValue(before.get(c.entry.segments.join(".")), c.entry.value));
  const fileHits = conflicts.filter((c) => c.path !== overlay);
  if (fileHits.length) await app.loadTheme({ reload: true });
  const files = [];
  for (const c of fileHits) {
    const mine = app.themeDrafts.get(c.path);
    const theirs = c.currentBlobId ? await app.readFile(c.path, { reload: true }).catch(() => null) : null;
    files.push({ path: c.path, mine: mine?.text ?? null, theirs: theirs?.encoding === "text" ? theirs.text : null, blobId: c.currentBlobId });
  }
  const choice = await openDialog({
    title: t("conflict.dialog"),
    content: conflictView(t, { head: conflicts[0]?.headCommit ?? null, keys: keys.map((c) => labelOf(app, c.entry)), files }),
    actions: [
      { label: t("conflict.discard"), value: "discard" },
      { label: t("conflict.keep"), value: "keep", variant: "primary", submit: true },
    ],
    wide: true,
  });
  const keep = choice === "keep";
  if (!keep && settingsHit) app.discardSettings();
  for (const f of files) {
    const d = app.themeDrafts.get(f.path);
    if (!d) continue;
    if (!keep) app.themeDrafts.discard(f.path);
    // Keep: the draft now stands on their version, so publishing again is a
    // deliberate replacement of what is on the branch, not a blind one.
    else if (d.text !== undefined) app.themeDrafts.setText({ path: f.path, blobId: f.blobId }, f.theirs ?? "", d.text);
    else app.themeDrafts.setImage({ path: f.path, blobId: f.blobId }, d.base64);
  }
  app.changed();
  app.notify(t(keep ? "conflict.kept" : "conflict.discarded"));
  app.refresh();
}
