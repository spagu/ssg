// The MDDB configuration: https only, no private or loopback host, no
// credentials in the URL — unless EDITOR_MDDB_ALLOW_HTTP=true; the
// collections, filters and language from the site model, narrowed by
// EDITOR_MDDB_COLLECTION; every refusal names a variable, never a value.
import { test } from "node:test";
import assert from "node:assert/strict";
import { mddbConfigFrom, mddbConfigured, privateHost } from "../src/mddb-config.ts";
import type { SiteModel } from "../src/site-model.ts";
import { MDDB_KEY } from "./fake-mddb.ts";
import { MDDB_VARS, mddbOnlyEnv, send, world } from "./mddb-fixtures.ts";
import { PERM } from "../src/permissions.ts";

const model = (mddb: SiteModel["mddb"], defaultLanguage: string | null = "en") => ({ mddb, defaultLanguage }) as SiteModel;
const env = (over: Record<string, unknown> = {}) => ({ ...MDDB_VARS, EDITOR_MDDB_COLLECTION: "blog", ...over });
const problem = (e: Record<string, unknown>, m: SiteModel | null = null) => {
  const r = mddbConfigFrom(e, m);
  return r.ok ? null : r.problem;
};

test("private and loopback hosts, by address and by name", () => {
  for (const h of ["127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1", "198.18.0.1", "224.0.0.1",
    "[::1]", "[::]", "[fd00::1]", "[fc00::1]", "[fe80::1]", "[::ffff:7f00:1]", "[::7f00:1]", "localhost", "db.localhost", "mddb.local", "x.internal", "box.lan", "a.home.arpa", "mddb", "mddb.local."]) {
    assert.equal(privateHost(h), true, h);
  }
  for (const h of ["mddb.example.com", "93.184.216.34", "172.32.0.1", "100.128.0.1", "[2001:db8::1]", "192.169.0.1"]) assert.equal(privateHost(h), false, h);
});

test("the URL: https only, no private host, no credentials, query or fragment; the switch allows http and private", () => {
  assert.equal(problem(env()), null);
  assert.match(problem(env({ EDITOR_MDDB_URL: "http://mddb.example.com" }))!, /must be https/);
  assert.match(problem(env({ EDITOR_MDDB_URL: "https://127.0.0.1:11023" }))!, /private or loopback/);
  assert.match(problem(env({ EDITOR_MDDB_URL: "https://[::1]/" }))!, /private or loopback/);
  assert.match(problem(env({ EDITOR_MDDB_URL: "https://u:p@mddb.example.com" }))!, /credentials/);
  assert.match(problem(env({ EDITOR_MDDB_URL: "https://mddb.example.com/?x=1" }))!, /query/);
  assert.match(problem(env({ EDITOR_MDDB_URL: "not a url" }))!, /not a URL/);
  assert.match(problem({})!, /not a URL/);
  assert.match(problem(env({ EDITOR_MDDB_URL: "ftp://mddb.example.com" }))!, /must be https/);
  assert.equal(problem(env({ EDITOR_MDDB_URL: "http://mddb:11023", EDITOR_MDDB_ALLOW_HTTP: "true" })), null);
  assert.equal(problem(env({ EDITOR_MDDB_URL: "https://10.0.0.5", EDITOR_MDDB_ALLOW_HTTP: true })), null);
  assert.match(problem(env({ EDITOR_MDDB_URL: "http://10.0.0.5", EDITOR_MDDB_ALLOW_HTTP: "yes" }))!, /must be https/);
  const r = mddbConfigFrom(env({ EDITOR_MDDB_URL: " https://mddb.example.com/base/// " }), null);
  assert.ok(r.ok && r.config.url === "https://mddb.example.com/base" && r.config.apiKey === MDDB_KEY);
  assert.equal(mddbConfigured({ EDITOR_MDDB_URL: "  " }), false);
  assert.equal(mddbConfigured({}), false);
});

test("collections from the site model, narrowed by EDITOR_MDDB_COLLECTION; language and new-article language", () => {
  const m = model({ enabled: true, collection: null, lang: "pl_PL", sources: [
    { collection: "blog", type: null, section: "blog" },
    { collection: "kb", type: "page", section: null, filter: { visibility: ["public"] } },
    { collection: null, type: null, section: null },
  ] });
  const all = mddbConfigFrom({ ...MDDB_VARS }, m);
  assert.ok(all.ok);
  assert.deepEqual(all.config.sources, [
    { collection: "blog", type: null, section: "blog", filter: {} },
    { collection: "kb", type: "page", section: null, filter: { visibility: ["public"] } },
  ]);
  assert.deepEqual([all.config.lang, all.config.newLang], ["pl_PL", "pl_PL"]);
  const kb = mddbConfigFrom({ ...MDDB_VARS, EDITOR_MDDB_COLLECTION: "kb", EDITOR_MDDB_LANG: "de" }, m);
  assert.ok(kb.ok && kb.config.sources.length === 1 && kb.config.sources[0].filter.visibility[0] === "public" && kb.config.lang === "de");
  const other = mddbConfigFrom({ ...MDDB_VARS, EDITOR_MDDB_COLLECTION: "news" }, m);
  assert.ok(other.ok && other.config.sources[0].collection === "news" && other.config.sources[0].type === null);
  const single = mddbConfigFrom({ ...MDDB_VARS }, model({ enabled: true, collection: "posts", sources: [] }, "fr"));
  assert.ok(single.ok && single.config.sources[0].collection === "posts" && single.config.lang === null && single.config.newLang === "fr");
  const fallback = mddbConfigFrom({ ...MDDB_VARS }, model({ enabled: true, collection: "posts", sources: [] }, "?"));
  assert.ok(fallback.ok && fallback.config.newLang === "en" && fallback.config.apiKey === MDDB_KEY);
  const noKey = mddbConfigFrom({ ...MDDB_VARS, EDITOR_MDDB_API_KEY: " ", EDITOR_MDDB_COLLECTION: "x" }, null);
  assert.ok(noKey.ok && noKey.config.apiKey === null && noKey.config.newLang === "en");
});

test("refusals: no collection, a collection or type ssg would refuse, a language that is not one", () => {
  assert.match(problem({ ...MDDB_VARS })!, /EDITOR_MDDB_COLLECTION/);
  assert.match(problem({ ...MDDB_VARS }, model(null))!, /EDITOR_MDDB_COLLECTION/);
  assert.match(problem(env({ EDITOR_MDDB_COLLECTION: "a b" }))!, /not a plain name/);
  assert.match(problem({ ...MDDB_VARS }, model({ enabled: true, collection: null, sources: [{ collection: "c", type: "note", section: null }] }))!, /page or post/);
  assert.match(problem(env({ EDITOR_MDDB_LANG: "english!" }))!, /language code/);
});

test("through the worker: a refused configuration is 503 mddb_not_configured naming the variable, never the key", async () => {
  const w = await world({ git: false });
  const res = await send(w, mddbOnlyEnv({ EDITOR_MDDB_URL: "http://127.0.0.1:11023" }), "/api/content", [PERM.read]);
  const body = await res.text();
  assert.equal(res.status, 503);
  assert.equal(JSON.parse(body).error, "mddb_not_configured");
  assert.match(JSON.parse(body).detail, /EDITOR_MDDB_URL/);
  assert.ok(!body.includes(MDDB_KEY));
  assert.equal(w.db.requests.length, 0, "nothing was sent");
  const path = await send(w, mddbOnlyEnv({ EDITOR_MDDB_URL: "https://192.168.0.2" }), `/api/content/${encodeURIComponent("mddb:blog/en/x")}`, [PERM.read]);
  assert.equal(path.status, 503);
});

test("site-keys: mddb.lang and each source's filter, only when set", async () => {
  const { siteKeys } = await import("../src/site-keys.ts");
  const k = siteKeys({ mddb: { lang: "en_US", sources: [{ collection: "kb", filter: { visibility: ["public", null], n: [1] } }, { collection: "blog" }] } });
  assert.deepEqual(k.mddb, { enabled: false, collection: null, lang: "en_US", sources: [
    { collection: "kb", type: null, section: null, filter: { visibility: ["public", ""], n: ["1"] } },
    { collection: "blog", type: null, section: null },
  ] });
  assert.throws(() => siteKeys({ mddb: { sources: [{ filter: { a: "x" } }] } }), /filter\.a is not a list/);
});
