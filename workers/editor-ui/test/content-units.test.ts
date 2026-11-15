// The small parts of the content operations on their own: where an article
// sits and its language, the meta cache, the commit author, the conflict's
// "theirs" within the subrequest budget, path refusals for odd input.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, ORIGIN, makeEnv, request, run, site } from "./fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { languageOf, sectionOf } from "../src/content-place.ts";
import type { SiteModel } from "../src/site-model.ts";
import { MAX_CACHED, cachedMeta, clearMetaCache, decodeText, metaOf, remember } from "../src/content-meta.ts";
import { SERVICE_DOMAIN, authorOf } from "../src/author.ts";
import type { Caller } from "../src/gate.ts";
import { commitUrl, conflictOf } from "../src/change-publish.ts";
import type { ContentScope } from "../src/content-scope.ts";
import { pathError } from "../src/content-scope.ts";
import { SUBREQUESTS } from "../src/repo.ts";
import { RepoError } from "../portable/repo-provider.ts";

const model = (over: Partial<SiteModel> = {}): SiteModel => ({
  contentRoots: ["content/blog", "content/blog/de", "docs"], sections: [{ path: "content/blog/posts", type: "post" }],
  mediaDir: null, staticDir: null, languages: ["en", "de", "fr"], defaultLanguage: "en", shortcodes: [], mddb: null, problems: [],
  languageSections: { de: "de", "/fr/blog/": " fr ", "fr": "", home: "pl", "": "x" },
  head: "h", branch: "main", configFile: ".ssg.yaml", overlay: false, overrides: { contentDirs: false, mediaDir: false }, ...over,
});

test("sectionOf: the deepest section or content folder; languageOf: own, longest language_sections prefix, default", () => {
  const m = model();
  assert.deepEqual(sectionOf("content/blog/posts/a/b.md", m), { section: "content/blog/posts", type: "post" });
  assert.deepEqual(sectionOf("content/blog/de/x.md", m), { section: "content/blog/de", type: null });
  assert.deepEqual(sectionOf("content/blog/de/x.md", { ...m, contentRoots: ["content/blog/de", "content/blog"] }), { section: "content/blog/de", type: null });
  assert.deepEqual(sectionOf("docs/x.md", { ...m, contentRoots: ["docs", "content/blog"] }), { section: "docs", type: null });
  assert.equal(languageOf("content/blog/x.md", "it", m), "it");
  assert.equal(languageOf("content/blog/de/x.md", null, { ...m, contentRoots: ["content/blog"] }), "de");
  assert.equal(languageOf("content/blog/fr/blog/x.md", null, m), "fr");
  assert.equal(languageOf("content/blog/fr/x.md", null, m), "en", "an empty value falls back to the default");
  assert.equal(languageOf("content/blog/x.md", null, m), "en", "home is the site root, not a folder");
  assert.equal(languageOf("docs/x.md", null, { ...m, defaultLanguage: null }), null);
});

test("meta: title fallback outside code fences, the cache keeps at most MAX_CACHED versions, decodeText refuses non-UTF-8", () => {
  assert.equal(metaOf("```\n# not this\n~~~\n# nor this\n```\n# This\n").title, "This");
  assert.deepEqual(metaOf("+++\ntitle = 'x'\n+++\n"), { title: null, date: null, draft: false, lang: null, format: "toml" });
  clearMetaCache();
  const meta = metaOf("# a");
  for (let i = 0; i <= MAX_CACHED; i++) remember(`b${i}`, meta);
  remember("b5", meta);
  assert.equal(cachedMeta("b0"), undefined);
  assert.equal(cachedMeta(`b${MAX_CACHED}`), meta);
  clearMetaCache();
  assert.equal(decodeText(new Uint8Array([0xc3])), null);
  assert.equal(decodeText(new Uint8Array([0xef, 0xbb, 0xbf, 0x61])), "﻿a", "the BOM is kept");
});

const caller = (kind: "user" | "service", id: string, email: string): Caller => ({ kind, user: { id, email }, perms: [], sessionId: "s", expiresAt: "" });

test("authorOf: a person by e-mail, a service account by id under .invalid; an unusable one refused", () => {
  assert.deepEqual(authorOf(caller("user", "u1", "ada@example.com")), { name: "ada@example.com", email: "ada@example.com" });
  assert.deepEqual(authorOf(caller("service", "sa 1<x>", "")), { name: "service account sa 1 x", email: `sa-1-x-@${SERVICE_DOMAIN}` });
  assert.throws(() => authorOf(caller("service", "", "")), (e: unknown) => e instanceof RepoError && e.code === "invalid_author");
  assert.throws(() => authorOf(caller("user", "u1", "")), (e: unknown) => e instanceof RepoError && e.code === "invalid_author");
  assert.equal(commitUrl({ provider: "github", apiBase: "", owner: "o", repo: "r", branch: null, token: "" }, "c"), "https://github.com/o/r/commit/c");
});

test("conflictOf: theirs within the budget; unread for budget, bytes that are not text, a failing read", async () => {
  let used = 0;
  const reads: Record<string, () => Uint8Array> = {
    "a.md": () => new TextEncoder().encode("theirs"),
    "bin.md": () => new Uint8Array([0xff]),
    "gone.md": () => { throw new RepoError("not_found"); },
    "boom.md": () => { throw new Error("boom"); },
  };
  const scope = { repo: { provider: { used: () => used, read: async (_ref: string, p: string) => ({ path: p, blobId: "v", bytes: reads[p]() }) } } } as unknown as ContentScope;
  const conflict = (paths: string[]) => new RepoError("conflict", { conflicts: paths.map((path) => ({ path, currentBlobId: path === "new.md" ? null : "v2", headCommit: "c" })) });
  const files = (await conflictOf(scope, conflict(["a.md", "bin.md", "gone.md", "new.md"]), new Map([["a.md", "mine"]]))).extra.files;
  assert.deepEqual(files, [
    { path: "a.md", yours: "mine", theirVersion: "v2", theirs: "theirs" },
    { path: "bin.md", yours: null, theirVersion: "v2", theirs: null, theirsUnread: "not_text" },
    { path: "gone.md", yours: null, theirVersion: "v2", theirs: null, theirsUnread: "unavailable" },
    { path: "new.md", yours: null, theirVersion: null, theirs: null },
  ]);
  used = SUBREQUESTS;
  assert.deepEqual((await conflictOf(scope, conflict(["a.md"]), new Map())).extra.files, [{ path: "a.md", yours: null, theirVersion: "v2", theirs: null, theirsUnread: "budget" }]);
  used = 0;
  await assert.rejects(conflictOf(scope, conflict(["boom.md"]), new Map()), /boom/);
});

test("pathError: input that is no string, a reason without its own sentence", () => {
  assert.deepEqual(pathError(5, "not_a_string").extra, { path: null, reason: "not_a_string" });
  assert.equal(pathError("", "empty").detail, "not a valid repository path");
});

test("a request without a content type is refused; a form edit whose read fails passes the host's error", async () => {
  const gh = await site();
  const P = "content/blog/posts/hello.md";
  const body = new TextEncoder().encode(JSON.stringify({ message: "x", changes: [] }));
  const r = await request(BASE + "/api/changes", [PERM.read, PERM.publish], { method: "POST", body, headers: { origin: ORIGIN } });
  assert.equal((await run(r, makeEnv(), gh)).status, 415);
  const sha = gh.store.blobIdAt("main", P);
  gh.override = (method, path) => (method === "GET" && path === `/git/blobs/${sha}` ? Response.json({ message: "x" }, { status: 500 }) : undefined);
  const edit = { message: "x", changes: [{ op: "put", path: P, version: gh.store.blobIdAt("main", P), body: "y" }] };
  const res = await run(await request(BASE + "/api/changes", [PERM.read, PERM.publish], { method: "POST", body: JSON.stringify(edit), headers: { origin: ORIGIN, "content-type": "application/json" } }), makeEnv(), gh);
  assert.deepEqual([res.status, (await res.json()).error], [502, "provider_error"]);
  gh.override = null;
  const odd = { message: "x", changes: [{ op: "put", path: 5, version: null, text: "x" }] };
  const bad = await run(await request(BASE + "/api/changes", [PERM.read, PERM.publish], { method: "POST", body: JSON.stringify(odd), headers: { origin: ORIGIN, "content-type": "application/json" } }), makeEnv(), gh);
  assert.deepEqual(await bad.json(), { error: "invalid_path", detail: "not a valid repository path", path: null, reason: "not_a_string" });
});
