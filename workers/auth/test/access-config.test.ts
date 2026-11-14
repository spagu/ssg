// GO-139: switching Access on. Both variables or neither; one alone, or a
// value that cannot be right, is a configuration error the routes name (503)
// — never a half-open method. Off, its routes do not exist (404).
import { test } from "node:test";
import assert from "node:assert/strict";
import { accessLogoutUrl, accessLookup, configuredLogoutUrl, teamHost } from "../functions/api/auth/_access-config.ts";
import type { AccessSetup } from "../functions/api/auth/_access-config.ts";
import { signInMethods } from "../functions/api/auth/_provider-methods.ts";
import { onRequestGet as linkGet, onRequestPost as linkPost } from "../functions/auth/access/link.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { call, makeEnv, useClock } from "./fakes.ts";
import { ACCESS_ENV, AUD, FakeAccess, accessEnv, enter, mint, where } from "./access-fake.ts";

test("teamHost: the team name or its cloudflareaccess.com host, nothing else", () => {
  for (const ok of ["myteam", "MyTeam", "myteam.cloudflareaccess.com", "https://myteam.cloudflareaccess.com/", " my-team "]) assert.match(teamHost(ok), /^my-?team\.cloudflareaccess\.com$/, ok);
  for (const bad of ["", "evil.example.com", "myteam.cloudflareaccess.com.evil.example.com", "http://myteam.cloudflareaccess.com", "my_team", "-x", "a.b", "myteam/../x"]) assert.equal(teamHost(bad), "", bad);
});

test("accessLookup: absent, enabled (with its switches), and every way it is misconfigured", () => {
  const look = (over: Partial<Env>) => accessLookup(over as Env);
  assert.deepEqual(look({}), { kind: "absent" });
  assert.deepEqual(look({ AUTH_ACCESS_TEAM: " ", AUTH_ACCESS_AUD: "" }), { kind: "absent" });
  assert.deepEqual(look(ACCESS_ENV), { kind: "enabled", setup: { host: "myteam.cloudflareaccess.com", aud: AUD, linkByEmail: false, autoCreate: false, defaultRole: "" } });
  const on = look({ ...ACCESS_ENV, AUTH_ACCESS_LINK_BY_EMAIL: "ON", AUTH_ACCESS_AUTO_CREATE: " on ", AUTH_ACCESS_DEFAULT_ROLE: " viewer " }) as { setup: AccessSetup };
  assert.deepEqual([on.setup.linkByEmail, on.setup.autoCreate, on.setup.defaultRole], [true, true, "viewer"]);
  assert.equal((look({ ...ACCESS_ENV, AUTH_ACCESS_LINK_BY_EMAIL: "off", AUTH_ACCESS_DEFAULT_ROLE: "viewer" }) as { setup: AccessSetup }).setup.defaultRole, "", "a role without auto-create is ignored");
  const problems: [Partial<Env>, RegExp][] = [
    [{ AUTH_ACCESS_TEAM: "myteam" }, /AUTH_ACCESS_TEAM is set but AUTH_ACCESS_AUD is not/],
    [{ AUTH_ACCESS_AUD: AUD }, /AUTH_ACCESS_AUD is set but AUTH_ACCESS_TEAM is not/],
    [{ ...ACCESS_ENV, AUTH_ACCESS_TEAM: "evil.example.com" }, /must be your team name/],
    [{ ...ACCESS_ENV, AUTH_ACCESS_AUD: "short" }, /Application Audience/],
    [{ ...ACCESS_ENV, AUTH_ACCESS_AUD: `${AUD} x` }, /Application Audience/],
    [{ ...ACCESS_ENV, AUTH_ACCESS_LINK_BY_EMAIL: "yes" }, /AUTH_ACCESS_LINK_BY_EMAIL must be/],
    [{ ...ACCESS_ENV, AUTH_ACCESS_AUTO_CREATE: "true" }, /AUTH_ACCESS_AUTO_CREATE must be/],
    [{ ...ACCESS_ENV, AUTH_ACCESS_AUTO_CREATE: "on" }, /needs AUTH_ACCESS_DEFAULT_ROLE/],
  ];
  for (const [over, re] of problems) {
    const l = look(over);
    assert.equal(l.kind, "misconfigured", JSON.stringify(over));
    assert.match((l as { problem: string }).problem, re);
  }
});

test("the method: listed after the others with its form origin when on; absent or misconfigured → not listed, the latter logged", () => {
  assert.deepEqual(signInMethods(makeEnv(ACCESS_ENV as Partial<Env>)).map((m) => [m.id, m.formOrigin ?? null]), [["password", null], ["access", "https://myteam.cloudflareaccess.com"]]);
  assert.deepEqual(signInMethods(makeEnv()).map((m) => m.id), ["password"]);
  const quiet = console.error;
  const logged: string[] = [];
  console.error = (m: string) => logged.push(m);
  try {
    assert.deepEqual(signInMethods(makeEnv({ AUTH_ACCESS_TEAM: "myteam" } as Partial<Env>)).map((m) => m.id), ["password"]);
  } finally {
    console.error = quiet;
  }
  assert.match(logged.join("\n"), /Cloudflare Access is not offered: AUTH_ACCESS_TEAM is set but AUTH_ACCESS_AUD is not/);
  assert.equal(accessLogoutUrl({ host: "myteam.cloudflareaccess.com" } as AccessSetup), "https://myteam.cloudflareaccess.com/cdn-cgi/access/logout");
  assert.equal(configuredLogoutUrl(makeEnv(ACCESS_ENV as Partial<Env>)), "https://myteam.cloudflareaccess.com/cdn-cgi/access/logout");
  assert.equal(configuredLogoutUrl(makeEnv()), null);
});

test("routes: absent → 404 not_found; half-configured → 503 naming the problem; Access as the only method is a working site", async () => {
  useClock();
  const fake = new FakeAccess().install();
  const quiet = console.error;
  console.error = () => {};
  try {
    const token = await mint();
    for (const [handler, method] of [[null, "GET"], [linkGet, "GET"], [linkPost, "POST"]] as const) {
      const run = (env: Env) => (handler ? call(handler, env, "/auth/access/link", { method, ...(method === "POST" ? { form: {} } : {}) }) : enter(env, token));
      const absent = await run(makeEnv());
      assert.deepEqual([absent.status, await absent.json()], [404, { error: "not_found" }], `${method} absent`);
      const half = await run(makeEnv({ AUTH_ACCESS_AUD: AUD } as Partial<Env>));
      assert.equal(half.status, 503);
      const body = await half.json();
      assert.equal(body.error, "access_misconfigured");
      assert.match(body.hint, /AUTH_ACCESS_TEAM/);
    }
    // Password off, no provider, Access on: the site works (Access is a way in).
    const only = accessEnv({ AUTH_PASSWORD_LOGIN: "off" });
    assert.equal(where(await enter(only, token)), "/auth/login?error=not_invited");
    // Password off and Access half-configured: nobody could sign in → named.
    const none = await enter(makeEnv({ AUTH_PASSWORD_LOGIN: "off", AUTH_ACCESS_TEAM: "myteam" } as Partial<Env>), token);
    assert.equal(none.status, 503);
    assert.match((await none.json()).hint, /Cloudflare Access \(docs\/ACCESS.md\)/);
  } finally {
    console.error = quiet;
    fake.restore();
  }
});
