// GO-139: auto-provisioning. Off by default (an unknown Access user gets the
// "not invited" page and no row); on, with a harmless default role, an
// account with that ONE role; and never an administrator — the method refuses
// to start (503) for owner, a wildcard, or any sensitive permission.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createAccessAccount, defaultRole } from "../functions/api/auth/_access-provision.ts";
import { onRequestGet as me } from "../functions/api/auth/me/permissions.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { Browser, T0, call, useClock } from "./fakes.ts";
import { seedUser } from "./seed.ts";
import { before, count, events } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, accessEnv, enter, errorCode, mint, where } from "./access-fake.ts";

const AUTO = { AUTH_ACCESS_AUTO_CREATE: "on", AUTH_ACCESS_DEFAULT_ROLE: "newsletter-viewer" };

test("auto-create off (default): an unknown Access user is refused and leaves no row", async () => {
  useClock();
  const env = accessEnv({ AUTH_ACCESS_DEFAULT_ROLE: "newsletter-viewer" }); // a role alone does nothing
  const fake = new FakeAccess().install();
  try {
    assert.equal(errorCode(await enter(env, await mint({ sub: "new", email: "new@example.com" }))), "not_invited");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM users"), 0);
  } finally {
    fake.restore();
  }
});

test("auto-create on: an account with the default role only, Access linked, audited; signed in", async () => {
  useClock();
  const env = accessEnv(AUTO);
  const fake = new FakeAccess().install();
  try {
    const browser = new Browser();
    const res = await enter(env, await mint({ sub: "new", email: "New@example.com" }), { browser, next: "/x" });
    assert.equal(where(res), "/x");
    const row = (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { get(): Record<string, unknown> } } }).sqlite
      .prepare("SELECT u.id, u.email, u.password_hash, i.subject FROM users u JOIN identities i ON i.user_id = u.id")
      .get();
    assert.deepEqual([row.email, row.password_hash, row.subject], ["new@example.com", null, "new"]);
    assert.deepEqual((await (await call(me, env, "/api/auth/me/permissions", { browser })).json()).token, ["newsletter:read"]);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM grants"), 1);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM group_members"), 0);
    const created = events(env, "account_created")[0];
    assert.deepEqual([created.user_id, created.method, created.detail.role, created.detail.email], [row.id, "access", "newsletter-viewer", "new@example.com"]);
    // An existing account by address is still not entered without the other opt-in.
    await seedUser(env, "old@example.com");
    assert.equal(errorCode(await enter(env, await mint({ sub: "old", email: "old@example.com" }))), "access_not_linked");
  } finally {
    fake.restore();
  }
});

test("auto-create refuses to start (503) for a default role that could make an administrator, or none at all", async () => {
  useClock();
  const fake = new FakeAccess().install();
  const quiet = console.error;
  const logged: string[] = [];
  console.error = (m: string) => logged.push(m);
  try {
    const cases: [string, string[] | null, RegExp][] = [
      ["owner", null, /is the owner role/],
      ["everything", ["*"], /holds \*/],
      ["all-newsletter", ["newsletter:*"], /holds newsletter:\*/],
      ["inviter", ["auth:invite", "comments:read"], /holds auth:invite/],
      ["eraser", ["newsletter:erase"], /holds newsletter:erase/],
      ["missing", null, /is not a role here/],
    ];
    for (const [name, perms, hint] of cases) {
      const env = accessEnv({ AUTH_ACCESS_AUTO_CREATE: "on", AUTH_ACCESS_DEFAULT_ROLE: name });
      await ensureSchema(env);
      if (perms) await createRole(env, { name, description: "", permissions: perms }, T0);
      const res = await enter(env, await mint({ sub: "n", email: "n@example.com" }));
      assert.equal(res.status, 503, name);
      const body = await res.json();
      assert.equal(body.error, "access_misconfigured");
      assert.match(body.hint, hint);
      assert.equal(count(env, "SELECT COUNT(*) AS n FROM users"), 0);
    }
    const noRole = await enter(accessEnv({ AUTH_ACCESS_AUTO_CREATE: "on" }), await mint());
    assert.equal(noRole.status, 503);
    assert.match((await noRole.json()).hint, /needs AUTH_ACCESS_DEFAULT_ROLE/);
    assert.ok(logged.some((m) => m.includes("refused to start")));
  } finally {
    console.error = quiet;
    fake.restore();
  }
});

test("createAccessAccount: a concurrent creation of the same address or identity loses without leaving half an account", async () => {
  useClock();
  const env = accessEnv(AUTO);
  const fake = new FakeAccess().install();
  try {
    await ensureSchema(env);
    const r = (await defaultRole(env, { host: "h", aud: "a", linkByEmail: false, autoCreate: true, defaultRole: "newsletter-viewer" })) as { roleId: string };
    const id = { subject: "s1", email: "race@example.com", emailVerified: true, name: "" };
    assert.ok(await createAccessAccount(env, id, r.roleId, T0));
    assert.equal(await createAccessAccount(env, { ...id, subject: "s2" }, r.roleId, T0), null, "the address is taken");
    assert.equal(await createAccessAccount(env, { ...id, email: "other@example.com" }, r.roleId, T0), null, "the identity is taken");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM users"), 1);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM grants"), 1);
    // Through the route: the address appears between the check and the insert.
    before(env, /FROM invitations WHERE email = \?/, () => {
      (env.AUTH_DB as unknown as { sqlite: { exec(s: string): void } }).sqlite.exec(
        "INSERT INTO users (id, email, display_name, status, must_change_password, created_at, updated_at) VALUES ('x', 'late@example.com', '', 'active', 0, 'a', 'a')",
      );
    });
    assert.equal(errorCode(await enter(env, await mint({ sub: "late", email: "late@example.com" }))), "account_exists");
    await assert.rejects(createAccessAccount({ ...env, AUTH_DB: { ...env.AUTH_DB, batch: () => Promise.reject(new Error("D1_ERROR: disk")) } } as unknown as Env, id, r.roleId, T0), /disk/);
  } finally {
    fake.restore();
  }
});
