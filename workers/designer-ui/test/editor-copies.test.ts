// Code shared with editor-ui (README of the round, decision 12): the source is
// workers/editor-ui, and designer-ui holds byte-identical copies, so an edit
// there turns this test red here instead of drifting silently. Copy again,
// never edit a copy. Skipped, with the reason, when this template was
// scaffolded alone (as the auth guard does, #423).
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { registerHooks } from "node:module";

// COPIES: copy (relative to this file) → source (relative to workers/editor-ui/).
export const COPIES: Record<string, string> = {
  // The repository layer and the path policy (GO-168), whole.
  "../portable/path-policy.ts": "portable/path-policy.ts",
  "../portable/repo-config.ts": "portable/repo-config.ts",
  "../portable/repo-github-commit.ts": "portable/repo-github-commit.ts",
  "../portable/repo-github-http.ts": "portable/repo-github-http.ts",
  "../portable/repo-github-tree.ts": "portable/repo-github-tree.ts",
  "../portable/repo-github.ts": "portable/repo-github.ts",
  "../portable/repo-provider.ts": "portable/repo-provider.ts",
  // The MCP transport (GO-176), whole — the endpoint of GO-181.
  "../portable/mcp-transport.ts": "portable/mcp-transport.ts",
  "../portable/mcp-methods.ts": "portable/mcp-methods.ts",
  // Generic files of editor-ui's skeleton (GO-169), unchanged.
  "../src/router.ts": "src/router.ts",
  "../src/module.ts": "src/module.ts",
  "../src/yaml-lite.ts": "src/yaml-lite.ts",
  "../src/yaml-scalar.ts": "src/yaml-scalar.ts",
  // The in-process fakes the repository tests run on.
  "./fake-github.ts": "test/fake-github.ts",
  "./fake-git-store.ts": "test/fake-git-store.ts",
};

const EDITOR = new URL("../../editor-ui/", import.meta.url);

// drifted lists every copy that is missing or differs from its source.
export function drifted(copies: Record<string, string>, editor: URL = EDITOR): string[] {
  const out: string[] = [];
  for (const [copy, source] of Object.entries(copies)) {
    const mine = new URL(copy, import.meta.url);
    if (!existsSync(mine)) out.push(`${copy} is missing — copy workers/editor-ui/${source} there`);
    else if (!readFileSync(mine).equals(readFileSync(new URL(source, editor)))) out.push(`${copy} differs from workers/editor-ui/${source} — copy it again, never edit a copy`);
  }
  return out;
}

const alone = !existsSync(EDITOR) && "workers/editor-ui is not next to this template (scaffolded alone): shared copies not checked";

test("copies of editor-ui's shared code are byte-identical to their sources", { skip: alone }, () => {
  assert.deepEqual(drifted(COPIES), []);
});

test("the editor-ui guard names a missing or drifted copy", { skip: alone }, () => {
  const problems = drifted({ "./no-such-copy.ts": "src/router.ts", "./fixtures.ts": "src/router.ts" });
  assert.equal(problems.length, 2);
  assert.match(problems[0], /missing/);
  assert.match(problems[1], /differs/);
});

// In this repository the sources' own suites run once more against THESE
// copies — a resolve hook sends their imports of the shared files here, as
// auth-copies.test.ts does for the verifier — so every copy is exercised in
// full, not only compared. The summary kit's suite (workers/module-summary.test.ts)
// runs against src/summary-kit.ts the same way. Scaffolded alone, there are
// no suites to run.
const SUITES = ["path-policy", "repo-config", "repo-provider", "repo-github-read", "repo-github-http", "repo-github-commit", "repo-github-budget", "yaml-lite", "mcp-transport"];
const KIT_SUITE = new URL("../../module-summary.test.ts", import.meta.url);
if (!alone && existsSync(KIT_SUITE)) {
  const editorTests = new URL("test/", EDITOR).href;
  const mine = (url: string): string | null => {
    for (const [copy, source] of Object.entries(COPIES)) {
      if (url === new URL(source, EDITOR).href && !source.startsWith("test/")) return new URL(copy, import.meta.url).href;
    }
    return url === new URL("../../module-summary-kit.ts", import.meta.url).href ? new URL("../src/summary-kit.ts", import.meta.url).href : null;
  };
  registerHooks({
    resolve(specifier, context, next) {
      const r = next(specifier, context);
      const fromSuite = context.parentURL?.startsWith(editorTests) || context.parentURL === KIT_SUITE.href;
      const to = fromSuite ? mine(r.url) : null;
      return to ? { url: to, shortCircuit: true } : r;
    },
  });
  for (const suite of SUITES) await import(new URL(`test/${suite}.test.ts`, EDITOR).href);
  await import(KIT_SUITE.href);
}
