// repo-config.ts: <PREFIX>_GIT_* for any prefix, refusal on anything missing, API host never configurable.
import { test } from "node:test";
import assert from "node:assert/strict";
import { parseRepo, repoConfigFrom } from "../portable/repo-config";
import { ENV } from "./repo-fixtures";
import { TOKEN } from "./fake-github";

test("a complete configuration; the API host comes from the provider table", () => {
  const r = repoConfigFrom(ENV, "EDITOR");
  assert.deepEqual(r, { ok: true, config: { provider: "github", apiBase: "https://api.github.com", owner: "acme", repo: "site", branch: "main", token: TOKEN } });
});

test("the prefix is the caller's (designer-ui reuses it); branch optional = default branch", () => {
  const r = repoConfigFrom({ DESIGNER_GIT_REPO: "https://GitHub.com/acme/site.git", DESIGNER_GIT_TOKEN: ` ${TOKEN} ` }, "DESIGNER");
  assert.ok(r.ok);
  assert.equal(r.config.branch, null);
  assert.equal(r.config.token, TOKEN);
  assert.equal(repoConfigFrom(ENV, "DESIGNER").ok, false);
});

test("refusals name the variable, never its value", () => {
  const cases: [Record<string, unknown>, string, RegExp][] = [
    [ENV, "editor", /invalid configuration prefix/],
    [{ ...ENV, EDITOR_GIT_REPO: "" }, "EDITOR", /EDITOR_GIT_REPO is not set/],
    [{ ...ENV, EDITOR_GIT_REPO: 42 }, "EDITOR", /EDITOR_GIT_REPO is not set/],
    [{ ...ENV, EDITOR_GIT_REPO: "acme/site" }, "EDITOR", /must look like/],
    [{ ...ENV, EDITOR_GIT_REPO: "gitlab.example.com/acme/site" }, "EDITOR", /only github.com/],
    [{ ...ENV, EDITOR_GIT_REPO: "api.github.com.example.com/acme/site" }, "EDITOR", /only github.com/],
    [{ ...ENV, EDITOR_GIT_BRANCH: "main..x" }, "EDITOR", /EDITOR_GIT_BRANCH is not a valid/],
    [{ ...ENV, EDITOR_GIT_TOKEN: undefined }, "EDITOR", /EDITOR_GIT_TOKEN is not set/],
    [{ ...ENV, EDITOR_GIT_TOKEN: "ghp_abc\r\nx-evil: 1" }, "EDITOR", /EDITOR_GIT_TOKEN is malformed/],
    [{ ...ENV, EDITOR_GIT_TOKEN: "short" }, "EDITOR", /EDITOR_GIT_TOKEN is malformed/],
  ];
  for (const [env, prefix, want] of cases) {
    const r = repoConfigFrom(env, prefix);
    assert.equal(r.ok, false);
    if (!r.ok) {
      assert.match(r.problem, want);
      assert.ok(!r.problem.includes("ghp_"), "no token value in a problem");
    }
  }
});

test("parseRepo accepts the documented forms and rejects odd owners and repos", () => {
  assert.deepEqual(parseRepo("github.com/acme/site"), { host: "github.com", owner: "acme", repo: "site" });
  assert.deepEqual(parseRepo("https://github.com/acme/site.js.git/"), { host: "github.com", owner: "acme", repo: "site.js" });
  for (const bad of ["github.com/-acme/site", "github.com/acme/..", "github.com/acme/s i", "http://github.com/acme/site", "github.com/acme/site/extra", "github.com/a_b/site"]) {
    assert.equal(parseRepo(bad), null, bad);
  }
});
