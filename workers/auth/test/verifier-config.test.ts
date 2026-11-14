// The portable verifier's (GO-135) configuration, its answers, its copy of `can`, and the copy
// guard helper consumers drop into their suites.
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync, copyFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import * as original from "../functions/api/auth/_can.ts";
import * as portable from "../portable/session-verifier.ts";
import { assertNoProblems, compareCopies } from "../portable/copy-guard.ts";
import { config } from "./verifier-fixtures.ts";

const { configFrom, denied, forbidden, loginRedirect, wantsPage } = portable;
const SOURCE = new URL("../portable/session-verifier.ts", import.meta.url);

test("configFrom: off, complete, and every half-configured set", () => {
  assert.equal(configFrom({}, "DEMO"), null, "nothing set: mode off");
  assert.equal(configFrom({ DEMO_AUTH_ISSUER: "  ", OTHER_AUTH_ISSUER: "x", DEMO_ADMIN_PASSWORD: "p" }, "DEMO"), null, "blank and foreign vars do not count");
  const full = { DEMO_AUTH_ISSUER: " https://example.com/api/auth ", DEMO_AUTH_PUBLIC_KEY: "{}", DEMO_AUTH_LOGIN_URL: "/auth/login" };
  assert.deepEqual(configFrom(full, "DEMO"), {
    issuer: "https://example.com/api/auth",
    audience: "",
    publicKey: "{}",
    jwksUrl: "",
    loginUrl: "/auth/login",
    configProblem: null,
  });
  const viaUrl = { DEMO_AUTH_ISSUER: "i", DEMO_AUTH_JWKS_URL: "/api/auth/jwks", DEMO_AUTH_LOGIN_URL: "/l", DEMO_AUTH_AUDIENCE: "example.com" };
  assert.equal(configFrom(viaUrl, "DEMO")?.configProblem, null);
  assert.equal(configFrom({ ...viaUrl, DEMO_AUTH_JWKS_URL: "https://example.com/api/auth/jwks" }, "DEMO")?.configProblem, null);
  const half: [Record<string, string>, RegExp][] = [
    [{ DEMO_AUTH_AUDIENCE: "example.com" }, /DEMO_AUTH_ISSUER is not set; DEMO_AUTH_LOGIN_URL is not set; set exactly one of DEMO_AUTH_PUBLIC_KEY and DEMO_AUTH_JWKS_URL/],
    [{ DEMO_AUTH_ISSUER: "i" }, /DEMO_AUTH_LOGIN_URL is not set/],
    [{ DEMO_AUTH_PUBLIC_KEY: "{}", DEMO_AUTH_LOGIN_URL: "/l" }, /^DEMO_AUTH_ISSUER is not set$/],
    [{ DEMO_AUTH_ISSUER: "i", DEMO_AUTH_PUBLIC_KEY: "{}" }, /^DEMO_AUTH_LOGIN_URL is not set$/],
    [{ DEMO_AUTH_ISSUER: "i", DEMO_AUTH_LOGIN_URL: "/l" }, /^set exactly one of/],
    [{ ...full, DEMO_AUTH_JWKS_URL: "/api/auth/jwks" }, /^set exactly one of/],
    [{ ...viaUrl, DEMO_AUTH_JWKS_URL: "http://example.com/jwks" }, /DEMO_AUTH_JWKS_URL must be https:\/\/ or a path/],
    [{ ...viaUrl, DEMO_AUTH_JWKS_URL: "//evil.example.com/jwks" }, /must be https/],
    [{ ...viaUrl, DEMO_AUTH_JWKS_URL: "/\\evil.example.com/jwks" }, /must be https/],
    [{ ...viaUrl, DEMO_AUTH_JWKS_URL: "api/auth/jwks" }, /must be https/],
  ];
  for (const [env, problem] of half) {
    const c = configFrom(env, "DEMO");
    assert.ok(c, JSON.stringify(env));
    assert.match(c.configProblem ?? "", problem, JSON.stringify(env));
  }
});

test("answers: page → 302 to login with a safe next; API → 401 JSON; 503s; 403", async () => {
  const c = config({ loginUrl: "/auth/login?lang=pl" });
  const page = (path: string) => new Request(`https://example.com${path}`, { headers: { accept: "text/html,application/xhtml+xml" } });
  const go = (path: string) => new URL(loginRedirect(page(path), c).headers.get("location") as string);
  assert.equal(go("/admin/?tab=queue").href, "https://example.com/auth/login?lang=pl&next=%2Fadmin%2F%3Ftab%3Dqueue");
  assert.equal(go("//evil.example.com/x").searchParams.get("next"), "/evil.example.com/x", "a path, never a protocol-relative host");
  assert.equal(go("/" + "a".repeat(600)).searchParams.get("next"), "/", "too long for the login page: the root");
  const res = loginRedirect(page("/admin/"), c);
  assert.equal(res.status, 302);
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(new URL(loginRedirect(page("/a"), config({ loginUrl: "https://login.example.com/in" })).headers.get("location") as string).host, "login.example.com");

  assert.equal(wantsPage(page("/")), true);
  assert.equal(wantsPage(new Request("https://example.com/", { method: "HEAD", headers: { accept: "text/html" } })), true);
  assert.equal(wantsPage(new Request("https://example.com/", { method: "POST", headers: { accept: "text/html" } })), false);
  assert.equal(wantsPage(new Request("https://example.com/api/x", { headers: { accept: "application/json" } })), false);
  assert.equal(wantsPage(new Request("https://example.com/api/x")), false);

  const api = new Request("https://example.com/api/x");
  const cases: [portable.Reason, number, Record<string, unknown>][] = [
    ["no_session", 401, { error: "unauthorized", reason: "no_session" }],
    ["expired", 401, { error: "unauthorized", reason: "expired" }],
    ["bad_signature", 401, { error: "unauthorized", reason: "invalid_session" }],
    ["wrong_kind", 401, { error: "unauthorized", reason: "invalid_session" }],
    ["keys_unavailable", 503, { error: "auth_unavailable" }],
    ["not_configured", 503, { error: "not_configured", detail: "the auth key or issuer is unusable" }],
  ];
  for (const [reason, status, body] of cases) {
    const r = denied(api, c, reason);
    assert.equal(r.status, status, reason);
    assert.deepEqual(await r.json(), body, reason);
    assert.equal(r.headers.get("content-type"), "application/json; charset=utf-8");
    assert.equal(r.headers.get("cache-control"), "no-store");
  }
  assert.equal(denied(api, c, "no_session").headers.get("www-authenticate"), 'Bearer realm="ssg-auth"');
  assert.equal(denied(api, c, "keys_unavailable").headers.get("retry-after"), "60");
  assert.deepEqual(await denied(api, config({ configProblem: "DEMO_AUTH_ISSUER is not set" }), "not_configured").json(), { error: "not_configured", detail: "DEMO_AUTH_ISSUER is not set" });
  assert.equal(denied(page("/admin/"), c, "expired").status, 302, "a page is sent to log in, whatever the reason");
  assert.equal(denied(page("/admin/"), c, "keys_unavailable").status, 503, "logging in again would not help");
  const f = forbidden("comments:moderate");
  assert.equal(f.status, 403);
  assert.deepEqual(await f.json(), { error: "forbidden", permission: "comments:moderate" });
});

test("can / validPermission: the code of _can.ts byte for byte, and the same answers", () => {
  const can = readFileSync(new URL("../functions/api/auth/_can.ts", import.meta.url), "utf8");
  const code = can.slice(can.indexOf("const PERMISSION"));
  const file = readFileSync(SOURCE, "utf8");
  const begin = file.indexOf("\n", file.indexOf("// ---- begin: the code of workers/auth/functions/api/auth/_can.ts")) + 1;
  assert.equal(file.slice(begin, file.indexOf("// ---- end: _can.ts ----")), code, "copy _can.ts from `const PERMISSION` to its end again");
  const perms = [["newsletter:send"], ["newsletter:*"], ["*"], [], ["Newsletter:Send"], ["*:send"]];
  for (const required of ["newsletter:send", "newsletter:*", "*", "", "x", "comments:read"]) {
    for (const p of perms) assert.equal(portable.can(p, required), original.can(p, required), `${p} / ${required}`);
    assert.equal(portable.validPermission(required), original.validPermission(required));
  }
  assert.equal(portable.can("x" as never, "a:b"), false);
});

test("the portable file: no imports, at most 200 lines", () => {
  const file = readFileSync(SOURCE, "utf8");
  assert.doesNotMatch(file, /^\s*import\s/m);
  assert.doesNotMatch(file, /\brequire\(|\bimport\(/);
  assert.ok(file.split("\n").length - 1 <= 200, `${file.split("\n").length - 1} lines`);
});

test("copy guard: identical, drifted, missing, and scaffolded alone", () => {
  const root = mkdtempSync(join(tmpdir(), "copy-guard-"));
  try {
    const portableDir = join(root, "workers/auth/portable");
    const testDir = join(root, "workers/demo/test");
    mkdirSync(portableDir, { recursive: true });
    mkdirSync(testDir, { recursive: true });
    copyFileSync(SOURCE, join(portableDir, "session-verifier.ts"));
    copyFileSync(SOURCE, join(testDir, "copy.ts"));
    writeFileSync(join(testDir, "drift.ts"), readFileSync(SOURCE, "utf8").replace("LEEWAY_SECONDS = 30", "LEEWAY_SECONDS = 3000"));
    const testUrl = pathToFileURL(join(testDir, "x.test.ts")).href;
    assert.deepEqual(compareCopies(testUrl, { "./copy.ts": "session-verifier.ts" }), { skip: false, problems: [] });
    const bad = compareCopies(testUrl, { "./drift.ts": "session-verifier.ts", "./gone.ts": "session-verifier.ts" });
    assert.equal(bad.skip, false);
    assert.match(bad.problems[0], /\.\/drift\.ts differs from workers\/auth\/portable\/session-verifier\.ts — copy it again/);
    assert.match(bad.problems[1], /\.\/gone\.ts is missing/);
    assert.throws(() => assertNoProblems(bad.problems), (e: Error) => e.message.split("\n").length === 2);
    assert.doesNotThrow(() => assertNoProblems([]));
    rmSync(join(root, "workers/auth"), { recursive: true });
    const alone = compareCopies(testUrl, { "./copy.ts": "session-verifier.ts" });
    assert.match(alone.skip as string, /scaffolded alone/);
    assert.deepEqual(alone.problems, []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
