// The site model: read from the branch's own configuration (ssg's file order,
// the overlay, only the keys it needs), cached per head, honest about what it
// cannot read, overridable by EDITOR_CONTENT_DIRS / EDITOR_MEDIA_DIR.
import { test } from "node:test";
import assert from "node:assert/strict";
import { GIT_VARS, SITE_YAML, site } from "./fixtures.ts";
import { FakeGitHub } from "./fake-github.ts";
import { repositoryOf, requireRepository } from "../src/repo.ts";
import { OVERRIDE_HINT, clearSiteCache, compose, loadSiteModel, requireSiteModel } from "../src/site-model.ts";
import type { SiteState } from "../src/site-model.ts";
import { CONFIG_FILES, merge, parse, readSiteConfig } from "../src/site-config.ts";
import { siteKeys } from "../src/site-keys.ts";
import { OpError } from "../src/ops.ts";

async function load(gh: FakeGitHub, env: Record<string, unknown> = {}): Promise<SiteState> {
  const r = repositoryOf({ ...GIT_VARS, ...env } as never, { fetch: gh.fetch });
  if (!r.ok) throw new Error(r.problem);
  return loadSiteModel(r.repo.provider, r.repo.config, "main", { ...GIT_VARS, ...env } as never);
}
const model = (s: SiteState) => {
  if (!s.ok) throw new Error(`${s.error}: ${s.detail}`);
  return s.model;
};
const failure = (s: SiteState) => {
  if (s.ok) throw new Error("expected a failure");
  return s;
};

test("the seeded site: content folders, sections, media, languages, shortcodes, mddb without its key", async () => {
  const m = model(await load(await site()));
  assert.deepEqual(m.contentRoots, ["content/blog", "docs"]);
  assert.deepEqual(m.sections, [{ path: "content/blog/pages", type: "page" }, { path: "content/blog/posts", type: "post" }, { path: "docs", type: "page" }]);
  assert.equal(m.mediaDir, "content/blog/media");
  assert.equal(m.staticDir, "static");
  assert.deepEqual([m.languages, m.defaultLanguage], [["en", "pl"], "en"]);
  assert.deepEqual(m.shortcodes, [{ name: "note", type: null, template: "note.html", title: null }]);
  assert.deepEqual(m.mddb, { enabled: false, collection: null, sources: [] });
  assert.ok(!JSON.stringify(m).includes("MDDB_KEY"), "mddb.api_key is never read into the model");
  assert.deepEqual([m.configFile, m.overlay, m.branch, m.overrides], [".ssg.yaml", false, "main", { contentDirs: false, mediaDir: false }]);
  assert.match(m.head, /^[0-9a-f]{40}$/);
  assert.deepEqual(m.problems, []);
});

test("cached per isolate by the branch head: one request when nothing changed, re-read after a commit", async () => {
  const gh = await site();
  await load(gh);
  const before = gh.requests.length;
  const again = model(await load(gh));
  assert.equal(gh.requests.length - before, 1, "only the head");
  await gh.store.commitFiles("main", { ".ssg.yaml": SITE_YAML.replace("source: blog", "source: news") });
  const after = model(await load(gh));
  assert.notEqual(after.head, again.head);
  assert.deepEqual(after.contentRoots, ["content/news", "docs"]);
  clearSiteCache();
});

test("ssg's file order; JSON read, TOML refused; the overlay merged as ssg merges it", async () => {
  assert.deepEqual(CONFIG_FILES.slice(0, 4), [".ssg.yaml", ".ssg.yml", ".ssg.toml", ".ssg.json"]);
  const yml = model(await load(await site({ ".ssg.yaml": null, ".ssg.yml": "source: a\n", "ssg.yaml": "source: b\n" })));
  assert.deepEqual([yml.configFile, yml.contentRoots], [".ssg.yml", ["content/a"]]);
  const json = model(await load(await site({ ".ssg.yaml": null, "ssg.json": JSON.stringify({ source: "j", content_dir: "src", title: "x" }) })));
  assert.deepEqual([json.configFile, json.contentRoots], ["ssg.json", ["src/j"]]);
  const toml = failure(await load(await site({ ".ssg.yaml": null, ".ssg.toml": "source = 'x'\n" })));
  assert.deepEqual([toml.error, toml.detail, toml.hint], ["site_config_unreadable", ".ssg.toml: it is TOML, which this worker does not read", OVERRIDE_HINT]);
  const overlay = JSON.stringify({ languages: ["de"], default_language: null, mddb: { collection: "posts" }, title: "ignored" });
  const o = model(await load(await site({ ".ssg.designer.json": overlay })));
  assert.deepEqual([o.overlay, o.languages, o.defaultLanguage, o.mddb], [true, ["de"], null, { enabled: false, collection: "posts", sources: [] }]);
  assert.deepEqual(merge({ a: { b: 1, c: 2 }, d: 1 }, { a: { c: 3 }, d: [1] }), { a: { b: 1, c: 3 }, d: [1] });
});

test("unreadable configurations are refused with what and where — never a value", async () => {
  const cases: [Record<string, string | null>, RegExp][] = [
    [{ ".ssg.yaml": "source: &s blog\n" }, /^\.ssg\.yaml: line 1 uses an anchor, alias or tag$/],
    [{ ".ssg.yaml": "source: blog\ninclude:\n  - base.yaml\n" }, /include:, and included files are not followed/],
    [{ ".ssg.yaml": null }, /^no configuration file \(\.ssg\.yaml, /],
    [{ ".ssg.yaml": null, ".ssg.json": "{\"source\": \"secret-ish" }, /^\.ssg\.json: it is not a JSON object$/],
    [{ ".ssg.yaml": null, ".ssg.json": "[1]" }, /it is not a JSON object/],
    [{ ".ssg.designer.json": "{bad" }, /^\.ssg\.designer\.json is not a JSON object$/],
    [{ ".ssg.designer.json": "[]" }, /is not a JSON object/],
    [{ ".ssg.yaml": "source: [a]\n" }, /^\.ssg\.yaml: source is not a single value$/],
    [{ ".ssg.yaml": "source: a\ncontent_sources: x\n" }, /content_sources is not a list/],
    [{ ".ssg.yaml": "source: a\nlanguage_sections: [x]\n" }, /language_sections is not a mapping/],
    [{ ".ssg.yaml": "source: a\nshortcodes:\n  - x\n" }, /shortcodes\[0\] is not a mapping/],
  ];
  for (const [files, detail] of cases) {
    const f = failure(await load(await site(files)));
    assert.equal(f.error, "site_config_unreadable", JSON.stringify(files));
    assert.match(f.detail, detail);
    assert.ok(!f.detail.includes("secret-ish"));
  }
});

test("not UTF-8: the config file or the overlay", async () => {
  const bad = new Uint8Array([0xff, 0xfe, 0x00]);
  const provider = (bytesFor: string) => ({
    read: async (_ref: string, path: string) => {
      if (path === bytesFor) return { path, bytes: bad, blobId: "b" };
      if (path === ".ssg.yaml") return { path, bytes: new TextEncoder().encode("source: a\n"), blobId: "a" };
      const { RepoError } = await import("../portable/repo-provider.ts");
      throw new RepoError("not_found");
    },
  });
  assert.deepEqual(await readSiteConfig(provider(".ssg.yaml") as never, "main"), { ok: false, file: ".ssg.yaml", detail: ".ssg.yaml is not UTF-8 text" });
  assert.deepEqual(await readSiteConfig(provider(".ssg.designer.json") as never, "main"), { ok: false, file: ".ssg.yaml", detail: ".ssg.designer.json is not UTF-8 text" });
});

test("a repository failure while reading is thrown, not mistaken for a missing file", async () => {
  const gh = await site();
  gh.override = (_m, path) => (path.startsWith("/contents") ? new Response("{}", { status: 500 }) : undefined);
  await assert.rejects(load(gh), { name: "RepoError", code: "provider_error" });
  gh.override = (_m, path) => (path.startsWith("/git/blobs") ? new Response("{}", { status: 500 }) : undefined);
  clearSiteCache();
  await assert.rejects(load(gh), { name: "RepoError" });
});

test("keys: defaults, content_dir, pages/posts paths, content_sources types, env references, outside the repository", () => {
  const k = siteKeys({
    content_dir: "./site/", source: "main", pages_path: "p", posts_path: "$POSTS", static_dir: "/abs",
    content_sources: [{ path: "notes", type: "post" }, { path: "../up" }, { name: "x" }, { path: "main-dup" }, { path: "notes" }],
    languages: [{ code: "en", name: "English" }, { code: "pl" }, { name: "no code" }],
    language_sections: { de: "de", home: null }, mddb: { enabled: true, collection: 7, sources: [{ collection: "c", type: "post", section: "blog" }] },
    shortcodes: [{ name: "a", type: "banner" }, { template: "nameless.html" }],
  });
  assert.deepEqual(k.contentRoots, ["site/main", "notes", "main-dup"]);
  assert.deepEqual(k.sections, [{ path: "site/main/p", type: "page" }, { path: "notes", type: "post" }, { path: "main-dup", type: "page" }]);
  assert.equal(k.staticDir, null);
  assert.deepEqual(k.languages, ["en", "pl"]);
  assert.deepEqual(k.languageSections, { de: "de", home: "" });
  assert.deepEqual(k.mddb, { enabled: true, collection: "7", sources: [{ collection: "c", type: "post", section: "blog" }] });
  assert.deepEqual(k.shortcodes, [{ name: "a", type: "banner", template: null, title: null }]);
  assert.deepEqual(k.problems.map((p) => p.code), ["env_reference", "outside_repository", "outside_repository", "outside_repository"]);
  assert.match(k.problems[0].detail, /^posts_path is set from an environment variable/);
  const none = siteKeys({});
  assert.deepEqual([none.contentRoots, none.mediaDir, none.staticDir, none.mddb], [[], null, "static", null]);
  const envSource = siteKeys({ source: "$SRC" });
  assert.deepEqual([envSource.contentRoots, envSource.problems[0].code], [[], "env_reference"]);
});

test("no content folder: refused with the hint, unless EDITOR_CONTENT_DIRS says where", async () => {
  const f = failure(await load(await site({ ".ssg.yaml": "title: x\n" })));
  assert.deepEqual([f.error, f.hint], ["no_content_dirs", OVERRIDE_HINT]);
  assert.match(f.detail, /names no content folder/);
  const m = model(await load(await site({ ".ssg.yaml": "title: x\n" }), { EDITOR_CONTENT_DIRS: "content/x" }));
  assert.deepEqual([m.contentRoots, m.mediaDir], [["content/x"], null]);
});

test("overrides: replace content folders and the media folder; usable over an unreadable file; checked", async () => {
  const gh = await site({ ".ssg.yaml": "source: |\n  blog\n" });
  const m = model(await load(gh, { EDITOR_CONTENT_DIRS: " content/a/ , docs ,", EDITOR_MEDIA_DIR: "static/img/" }));
  assert.deepEqual([m.contentRoots, m.mediaDir, m.configFile, m.overrides], [["content/a", "docs"], "static/img", null, { contentDirs: true, mediaDir: true }]);
  assert.deepEqual(m.problems.map((p) => p.code), ["site_config_unreadable"]);
  assert.deepEqual([m.languages, m.shortcodes], [[], []]);
  const overSeeded = model(await load(await site(), { EDITOR_MEDIA_DIR: "media" }));
  assert.deepEqual([overSeeded.contentRoots, overSeeded.mediaDir, overSeeded.languages], [["content/blog", "docs"], "media", ["en", "pl"]]);
  const bad = (env: Record<string, unknown>) => failure(compose({ ok: true, file: ".ssg.yaml", overlay: false, keys: siteKeys({ source: "a" }) }, env as never, "h", "main"));
  assert.match(bad({ EDITOR_CONTENT_DIRS: "../x" }).detail, /^EDITOR_CONTENT_DIRS: an entry is not a folder inside the repository \(traversal\)$/);
  assert.match(bad({ EDITOR_MEDIA_DIR: "/abs" }).detail, /^EDITOR_MEDIA_DIR: .*\(absolute\)$/);
  assert.deepEqual([bad({ EDITOR_MEDIA_DIR: "a,b" }).error, bad({ EDITOR_MEDIA_DIR: "a,b" }).detail], ["bad_override", "EDITOR_MEDIA_DIR: one folder only"]);
});

test("parse: by extension", () => {
  assert.deepEqual(parse("ssg.yml", "source: a\nother: b\n"), { source: "a" });
  assert.throws(() => parse("ssg.toml", ""), /TOML/);
});

test("repositoryOf / requireRepository: the provider, or the problem naming the variable", () => {
  assert.equal(repositoryOf(GIT_VARS as never, {}).ok, true);
  assert.deepEqual(repositoryOf({} as never, {}), { ok: false, problem: "EDITOR_GIT_REPO is not set" });
  assert.throws(() => requireRepository({} as never, {}), (err: unknown) => err instanceof OpError && err.code === "repo_not_configured" && err.status === 503);
  assert.equal(requireRepository(GIT_VARS as never, {}).config.repo, "site");
});

test("requireSiteModel: the model, or a 503 OpError carrying the detail and the hint", async () => {
  const gh = await site({ ".ssg.yaml": "source: |\n  blog\n" });
  const r = requireRepository(GIT_VARS as never, { fetch: gh.fetch });
  await assert.rejects(requireSiteModel(r.provider, r.config, "main", GIT_VARS as never), (err: unknown) =>
    err instanceof OpError && err.status === 503 && err.code === "site_config_unreadable" && err.detail === ".ssg.yaml: line 1 uses a multi-line block value (| or >)" && err.extra.hint === OVERRIDE_HINT);
  const env = { ...GIT_VARS, EDITOR_CONTENT_DIRS: "docs" };
  assert.deepEqual((await requireSiteModel(r.provider, r.config, "main", env as never)).contentRoots, ["docs"]);
});
