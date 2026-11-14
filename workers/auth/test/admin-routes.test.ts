// The route table (GO-133), generated from the files under functions/: every
// permission-checked route answers 401 without a session, 403 (naming the
// permission) to a session without it, and succeeds with it; every
// state-changing one refuses a request without the CSRF defence.
//
// Later features each keep their own generated route table (OWNERS below). The
// first test makes sure no route escapes all of them: a new route file must
// have a line here, or be claimed by exactly one owning table whose source
// names it — otherwise it fails.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { seedUser, signedIn } from "./seed.ts";

const ROOT = new URL("../functions/api/auth/", import.meta.url).pathname;
type Ids = { role: string; group: string; grant: string; member: string; other: string };

// One line per permission-checked route: the permission (null = any session),
// the URL params and the body of a request that should succeed.
const ROUTES: Record<string, { perm: string | null; params?: (i: Ids) => Record<string, string>; json?: (i: Ids) => unknown }> = {
  "GET permissions.ts": { perm: null },
  "GET me/permissions.ts": { perm: null },
  "GET module/index.ts": { perm: "auth:admin" },
  "GET admin/roles/index.ts": { perm: "auth:admin" },
  "POST admin/roles/index.ts": { perm: "auth:admin", json: () => ({ name: "new-role", permissions: ["newsletter:read"] }) },
  "GET admin/roles/[id].ts": { perm: "auth:admin", params: (i) => ({ id: i.role }) },
  "PUT admin/roles/[id].ts": { perm: "auth:admin", params: (i) => ({ id: i.role }), json: () => ({ description: "d" }) },
  "DELETE admin/roles/[id].ts": { perm: "auth:admin", params: (i) => ({ id: i.role }) },
  "GET admin/groups/index.ts": { perm: "auth:admin" },
  "POST admin/groups/index.ts": { perm: "auth:admin", json: () => ({ name: "New" }) },
  "GET admin/groups/[id]/index.ts": { perm: "auth:admin", params: (i) => ({ id: i.group }) },
  "PUT admin/groups/[id]/index.ts": { perm: "auth:admin", params: (i) => ({ id: i.group }), json: () => ({ description: "d" }) },
  "DELETE admin/groups/[id]/index.ts": { perm: "auth:admin", params: (i) => ({ id: i.group }) },
  "POST admin/groups/[id]/members/index.ts": { perm: "auth:admin", params: (i) => ({ id: i.group }), json: (i) => ({ userId: i.other }) },
  "DELETE admin/groups/[id]/members/[userId].ts": { perm: "auth:admin", params: (i) => ({ id: i.group, userId: i.member }) },
  "GET admin/grants/index.ts": { perm: "auth:admin" },
  "POST admin/grants/index.ts": { perm: "auth:admin", json: (i) => ({ roleId: i.role, userId: i.other }) },
  "DELETE admin/grants/[id].ts": { perm: "auth:admin", params: (i) => ({ id: i.grant }) },
  "GET admin/users/[id]/permissions.ts": { perm: "auth:admin", params: (i) => ({ id: i.member }) },
  "POST admin/users/[id]/revoke-sessions.ts": { perm: "auth:admin", params: (i) => ({ id: i.member }) },
};

// Session-only routes of GO-132 that check no permission and are covered by
// their own tests.
const GO132 = new Set(["bootstrap.ts", "jwks.ts", "login.ts", "logout-all.ts", "logout.ts", "password.ts", "refresh.ts", "session.ts"]);

// Route files covered by another generated route table: path prefix → the
// test file that owns them. `base` is the prefix that table's keys are
// relative to ("" = relative to functions/api/auth/, like here).
const OWNERS: { prefix: string; test: string; base: string }[] = [
  { prefix: "admin/invitations/", test: "go134-routes.test.ts", base: "" },
  { prefix: "admin/users/[id]/reset-link.ts", test: "go134-routes.test.ts", base: "" },
  { prefix: "forgot.ts", test: "go134-routes.test.ts", base: "" },
  { prefix: "invite.ts", test: "go134-routes.test.ts", base: "" },
  { prefix: "reset.ts", test: "go134-routes.test.ts", base: "" },
  { prefix: "resume.ts", test: "go134-routes.test.ts", base: "" },
  { prefix: "sessions/", test: "go134-routes.test.ts", base: "" },
  { prefix: "oauth/", test: "go151-routes.test.ts", base: "" },
  { prefix: "methods/", test: "go151-routes.test.ts", base: "" },
  { prefix: "mfa/", test: "mfa-routes.test.ts", base: "mfa/" },
  { prefix: "token.ts", test: "service-routes.test.ts", base: "" },
  { prefix: "admin/service-accounts/", test: "service-routes.test.ts", base: "" },
  { prefix: "admin/access-groups/", test: "access-routes.test.ts", base: "" },
  { prefix: "admin/users/index.ts", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "admin/users/[id]/index.ts", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "admin/users/[id]/status.ts", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "admin/sessions/", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "admin/events/", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "admin/status.ts", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "admin/matrix.ts", test: "admin-panel-routes.test.ts", base: "" },
  { prefix: "module/summary.ts", test: "admin-panel-routes.test.ts", base: "" },
];
const ownerOf = (file: string) => OWNERS.filter((o) => file === o.prefix || (o.prefix.endsWith("/") && file.startsWith(o.prefix)));

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return files(p);
    return n.startsWith("_") ? [] : [relative(ROOT, p)];
  });
}

type Handler = { method: string; file: string; fn: unknown };
async function handlers(): Promise<Handler[]> {
  const out: Handler[] = [];
  for (const file of files(ROOT).sort()) {
    if (GO132.has(file) || ownerOf(file).length) continue;
    const mod = await import(join(ROOT, file));
    for (const [name, fn] of Object.entries(mod)) {
      const m = /^onRequest(Get|Post|Put|Delete|Patch)$/.exec(name);
      if (m) out.push({ method: m[1].toUpperCase(), file, fn });
    }
  }
  return out;
}

const urlOf = (file: string, params: Record<string, string>) =>
  "/api/auth/" + file.replace(/(\/index)?\.ts$/, "").replace(/\[(\w+)\]/g, (_, k) => params[k]);

async function fixture() {
  useClock();
  const env = makeEnv();
  const member = await seedUser(env, "member@example.com");
  const other = await seedUser(env, "other@example.com");
  const role = (await createRole(env, { name: "reader", description: "", permissions: ["newsletter:read"] }, T0)) as Role;
  const group = (await createGroup(env, "Readers", "", T0)) as Group;
  await addMember(env, group.id, member, "x", T0);
  const grant = await addGrant(env, role.id, { userId: member }, "x", T0);
  const ids: Ids = { role: role.id, group: group.id, grant: (grant as { id: string }).id, member, other };
  return { env, ids };
}

test("route table: every permission-checked route file has exactly one line, and vice versa", async () => {
  const found = (await handlers()).map((h) => `${h.method} ${h.file}`).sort();
  assert.deepEqual(found, Object.keys(ROUTES).sort());
});

test("route tables: every route file is checked by exactly one table, which names it", () => {
  const ours = new Set(Object.keys(ROUTES).map((k) => k.split(" ")[1]));
  const unowned: string[] = [];
  for (const file of files(ROOT).sort()) {
    if (GO132.has(file) || ours.has(file)) continue;
    const owners = ownerOf(file);
    if (owners.length !== 1) {
      unowned.push(`${file} (${owners.length} owning tables)`);
      continue;
    }
    const { test: owner, base } = owners[0];
    const source = readFileSync(new URL(owner, import.meta.url), "utf8");
    if (!source.includes(file.slice(base.length))) unowned.push(`${file} (not named in ${owner})`);
  }
  assert.deepEqual(unowned, [], "add a line to ROUTES above, or to the owning table");
});

for (const h of await handlers()) {
  const key = `${h.method} ${h.file}`;
  const spec = ROUTES[key];
  if (!spec) continue;
  test(`route table: ${key} — no session 401, without ${spec.perm ?? "a session"} refused, with it 2xx`, async () => {
    const run = async (browser: Browser | undefined) => {
      const { env, ids } = await fixture();
      const b = browser ? await browserFor(env, browser) : undefined;
      const params = spec.params?.(ids) ?? {};
      const json = spec.json?.(ids);
      return call(h.fn, env, urlOf(h.file, params), { method: h.method, browser: b, params, ...(json !== undefined ? { json } : {}) });
    };
    const none = await run(undefined);
    assert.equal(none.status, 401);
    assert.deepEqual(await none.json(), { error: "not_authenticated" });
    if (spec.perm) {
      const without = await run(new Browser("lacking"));
      assert.equal(without.status, 403);
      assert.deepEqual(await without.json(), { error: "forbidden", permission: spec.perm });
    }
    const ok = await run(new Browser("holding"));
    assert.ok(ok.status >= 200 && ok.status < 300, `${key}: ${ok.status} ${await ok.clone().text()}`);
    if (h.method !== "GET") {
      const { env, ids } = await fixture();
      const b = await browserFor(env, new Browser("holding"));
      const params = spec.params?.(ids) ?? {};
      const res = await call(h.fn, env, urlOf(h.file, params), { method: h.method, browser: b, params, csrf: false, json: spec.json?.(ids) ?? {}, headers: { origin: "https://example.com" } });
      assert.equal(res.status, 403, `${key} without x-ssg-auth`);
      assert.equal((await res.json()).error, "csrf_check_failed");
      const cross = await call(h.fn, env, urlOf(h.file, params), { method: h.method, browser: b, params, json: spec.json?.(ids) ?? {}, headers: { origin: "https://evil.example.com" } });
      assert.equal(cross.status, 403, `${key} from another origin`);
    }
  });
}

// browserFor signs in "lacking" (newsletter:read only) or "holding"
// (auth:admin plus newsletter:read, so creating a reader role is no escalation).
async function browserFor(env: ReturnType<typeof makeEnv>, b: Browser): Promise<Browser> {
  const perms = b.ip === "holding" ? ["auth:admin", "newsletter:read"] : ["newsletter:read"];
  return (await signedIn(env, `${b.ip}@example.com`, perms)).browser;
}
