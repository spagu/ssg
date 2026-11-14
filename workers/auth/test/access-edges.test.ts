// GO-139, the remaining edges: a token without an address, a certs answer
// without keys, the login entry without `next`, an invitation that changes
// between finding and accepting it, and a database error that is not a
// duplicate.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createMapping } from "../functions/api/auth/_access-groups.ts";
import { onRequestGet as linkGet } from "../functions/auth/access/link.ts";
import { onRequestGet as loginGet } from "../functions/auth/login.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { T0, call, useClock } from "./fakes.ts";
import { invite } from "./go134.ts";
import { loggedIn, signedIn } from "./seed.ts";
import { before, count } from "./go151.ts";
import { FakeAccess, accessEnv, enter, errorCode, mint } from "./access-fake.ts";

const sql = (env: ReturnType<typeof accessEnv>, s: string) => env.AUTH_DB.sqlite.exec(s);

test("a token without an address: nothing to match (access_email_unusable); the link page shows a dash", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  try {
    assert.equal(errorCode(await enter(env, await mint({ sub: "no-mail", email: "" }, { email: undefined }))), "access_email_unusable");
    const { browser } = await loggedIn(env);
    const page = await (await call(linkGet, env, "/auth/access/link", { browser, csrf: false, headers: { "cf-access-jwt-assertion": await mint({ sub: "no-mail", email: "" }, { email: undefined }) } })).text();
    assert.match(page, /Cloudflare Access says you are —\./);
  } finally {
    fake.restore();
  }
});

test("a certs answer without a keys list is no keys; the login entry without next links /auth/access", async () => {
  useClock();
  const fake = new FakeAccess().install();
  const warn = console.warn;
  console.warn = () => {};
  try {
    fake.mode = "nokeys";
    assert.equal(errorCode(await enter(accessEnv(), await mint())), "access_unavailable");
    const html = await (await call(loginGet, accessEnv({ AUTH_AFTER_LOGIN_URL: "/home" }), "/auth/login", { csrf: false })).text();
    assert.match(html, /href="\/auth\/access" data-provider="access"/);
  } finally {
    console.warn = warn;
    fake.restore();
  }
});

test("an invitation withdrawn, or an account made for its address, between finding and accepting it: refused, nothing half-made", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    await invite(env, boss.browser, { email: "ines@example.com", delivery: "link" });
    before(env, /SELECT token_hash FROM invitations WHERE id/, () => sql(env, "UPDATE invitations SET revoked_at = 'x' WHERE email = 'ines@example.com'"));
    assert.equal(errorCode(await enter(env, await mint({ sub: "ines", email: "ines@example.com" }))), "invitation_invalid");
    await invite(env, boss.browser, { email: "otto@example.com", delivery: "link" });
    before(env, /SELECT token_hash FROM invitations WHERE id/, () =>
      sql(env, "INSERT INTO users (id, email, display_name, status, must_change_password, created_at, updated_at) VALUES ('o', 'otto@example.com', '', 'active', 0, 'a', 'a')"),
    );
    assert.equal(errorCode(await enter(env, await mint({ sub: "otto", email: "otto@example.com" }))), "account_exists");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
  } finally {
    fake.restore();
  }
});

test("the link route answers the configuration problem before anything else", async () => {
  const res = await call(linkGet, { ...accessEnv(), AUTH_DB: undefined } as never, "/auth/access/link", { csrf: false });
  assert.equal(res.status, 503);
});

test("createMapping: a database error other than a duplicate is not swallowed", async () => {
  const env = accessEnv();
  await ensureSchema(env);
  env.AUTH_DB.state.failOn = /INSERT INTO access_group_map/;
  await assert.rejects(createMapping(env, "g", "x", "boss", T0), /forced failure/);
});
