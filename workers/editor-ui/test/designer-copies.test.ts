// The panel's generic modules are shared with designer-ui (GO-179 wrote them
// there first): editor-ui holds byte-identical copies, so an edit on that
// side turns this test red here instead of drifting silently. Copy again,
// never edit a copy — a need the copies do not cover goes into a new
// editor-ui module (public/js/panel/hunks-view.js is one). Skipped, with the
// reason, when this template was scaffolded alone (as the auth guard does, #423).
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

// COPIES: copy (relative to this file) → source (relative to workers/designer-ui/).
export const COPIES: Record<string, string> = Object.fromEntries(
  [
    "core/vnode", "core/router", "core/i18n", "core/api", "core/prefs", "core/session", "core/diff", "core/format", "core/admin-nav",
    "panel/ui", "panel/dialog", "panel/publish-dialog", "panel/diff-view", "panel/conflict-view", "panel/revisions",
  ].map((m) => [`../public/js/${m}.js`, `public/js/${m}.js`]),
);

const DESIGNER = new URL("../../designer-ui/", import.meta.url);

// drifted lists every copy that is missing or differs from its source.
export function drifted(copies: Record<string, string>, designer: URL = DESIGNER): string[] {
  const out: string[] = [];
  for (const [copy, source] of Object.entries(copies)) {
    const mine = new URL(copy, import.meta.url);
    if (!existsSync(mine)) out.push(`${copy} is missing: copy workers/designer-ui/${source} there`);
    else if (!readFileSync(mine).equals(readFileSync(new URL(source, designer)))) out.push(`${copy} differs from workers/designer-ui/${source}: copy it again, never edit a copy`);
  }
  return out;
}

const alone = !existsSync(DESIGNER) && "workers/designer-ui is not next to this template (scaffolded alone): shared panel modules not checked";

test("copies of designer-ui's generic panel modules are byte-identical to their sources", { skip: alone }, () => {
  assert.deepEqual(drifted(COPIES), []);
});

test("the designer-ui guard names a missing or drifted copy", { skip: alone }, () => {
  const problems = drifted({ "./no-such-copy.js": "public/js/core/vnode.js", "./fixtures.ts": "public/js/core/vnode.js" });
  assert.equal(problems.length, 2);
  assert.match(problems[0], /missing/);
  assert.match(problems[1], /differs/);
});
