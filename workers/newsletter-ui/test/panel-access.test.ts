// GO-137, the panel's side: what an account sees (models/access.js), the
// session round trips (core/session.js), the API wrapper's 403-permission and
// 401-renew paths, and words for every key built at run time.
import { test } from "node:test";
import assert from "node:assert/strict";
import { classify, createApi } from "../public/js/core/api.js";
import { readMe, renew } from "../public/js/core/session.js";
import { ROUTES as VIEWS } from "../public/js/core/router.js";
import {
  ACTION_NEEDS, ALL, AUDIT_ACTIONS, VIEW_NEEDS, account, allows, auditWho, lacking, loginTarget, navFor, viewLacks,
} from "../public/js/models/access.js";
import en from "../public/js/strings/en.js";
import { DECLARED } from "../src/permissions.ts";
import { ROUTES } from "../src/routes.ts";

const NAV = ["dashboard", "subscribers", "import", "campaigns", "suppressions", "settings", "activity", "help"];
const me = (permissions: string[] | "all", extra: Record<string, unknown> = {}) => ({ mode: "auth", user: { id: "u1", email: "ada@example.com" }, permissions, loginUrl: "/auth/login", logoutUrl: "/api/auth/logout", refreshUrl: "/api/auth/refresh", ...extra });

test("password / Access: nothing is hidden", () => {
  assert.deepEqual(navFor(ALL, NAV), NAV);
  for (const action of Object.keys(ACTION_NEEDS)) assert.equal(allows(ALL, action), true, action);
  for (const view of Object.keys(VIEW_NEEDS)) assert.equal(viewLacks(ALL, view), null, view);
  assert.equal(account(ALL), null);
  assert.equal(loginTarget(ALL, "/x"), null);
});

test("a read-only account: the views to look at, no button that changes anything", () => {
  const reader = me(["newsletter:read"]);
  assert.deepEqual(navFor(reader, NAV), ["dashboard", "subscribers", "campaigns", "suppressions", "help"]);
  assert.deepEqual(Object.keys(ACTION_NEEDS).filter((a) => allows(reader, a)), []);
  assert.equal(viewLacks(reader, "campaignNew"), "newsletter:edit");
  assert.equal(viewLacks(reader, "activity"), "newsletter:settings");
});

test("an editor: drafts and tags, not sending, erasing or exporting", () => {
  const editor = me(["newsletter:read", "newsletter:edit"]);
  const can = Object.keys(ACTION_NEEDS).filter((a) => allows(editor, a));
  assert.deepEqual(can, ["save", "duplicate", "delete", "newCampaign", "fromPost", "editSubscriber", "unsubscribe"]);
  assert.equal(allows(editor, "newsletter:edit"), true, "a permission name works too");
});

test("several needs: the suppression import wants import AND suppress", () => {
  assert.equal(lacking(me(["newsletter:import"]), ACTION_NEEDS.importSuppressions), "newsletter:suppress");
  assert.equal(lacking(me(["newsletter:import", "newsletter:suppress"]), ACTION_NEEDS.importSuppressions), null);
  assert.equal(lacking(me([]), null), null);
  assert.equal(viewLacks(me([]), "nowhere"), null, "an unknown view needs nothing (notFound)");
});

test("every panel need is a permission module.json declares, every view has one", () => {
  for (const need of [...Object.values(ACTION_NEEDS), ...Object.values(VIEW_NEEDS)].flat()) if (need) assert.ok(DECLARED.includes(need), need);
  for (const [name] of VIEWS as [string][]) assert.ok(name in VIEW_NEEDS, name);
});

test("account and login target in auth mode", () => {
  assert.deepEqual(account(me(["*"])), { email: "ada@example.com", logoutUrl: "/api/auth/logout" });
  assert.deepEqual(account(me(["*"], { user: { id: "svc", email: "" } })), { email: "svc", logoutUrl: "/api/auth/logout" });
  assert.equal(account(me(["*"], { user: null })), null);
  assert.equal(loginTarget(me(["*"]), "/newsletter-admin/#/campaigns?x=1"), "/auth/login?next=%2Fnewsletter-admin%2F%23%2Fcampaigns%3Fx%3D1");
  assert.equal(loginTarget(me(["*"], { loginUrl: "/login?lang=pl" }), "/a"), "/login?lang=pl&next=%2Fa");
  assert.equal(loginTarget(me(["*"], { loginUrl: undefined }), "/a"), null);
});

test("the audit actions are the registry's, each with words", () => {
  const registry = ROUTES.map((r) => r.audit).filter((a): a is string => typeof a === "string").sort();
  assert.deepEqual([...AUDIT_ACTIONS].sort(), registry);
  for (const a of AUDIT_ACTIONS) assert.ok(Object.hasOwn(en, `audit.act.${a}`), a);
  for (const k of ["audit.mode.password", "audit.mode.access", "nav.activity", "error.permission", "set.auth.auth"]) assert.ok(Object.hasOwn(en, k), k);
  assert.equal(auditWho({ actor: { id: "u", email: "u@example.com" } }), "u@example.com");
  assert.equal(auditWho({ actor: { id: "u", email: "" } }), "u");
  assert.equal(auditWho({ actor: null }), null);
});

test("api: a 403 naming a permission is kind 'permission'; other 403s keep their kinds", () => {
  const e = classify(403, { error: "forbidden", permission: "newsletter:send" }, null);
  assert.deepEqual([e.kind, e.permission], ["permission", "newsletter:send"]);
  assert.equal(classify(403, { error: "forbidden" }, null).kind, "forbidden");
  assert.equal(classify(403, { error: "wrong Access audience" }, null).permission, null);
});

test("api: a 401 asks onUnauthorized once — retry on yes, the 401 stands on no", async () => {
  let n = 0;
  const answers = [401, 200];
  const fetch = async () => new Response(JSON.stringify({ ok: n }), { status: answers[n++] ?? 401, headers: { "content-type": "application/json" } });
  let asked = 0;
  const api = createApi({ base: "https://example.com/b/", fetch, onUnauthorized: async () => (asked++, true) });
  assert.deepEqual(await api.post("x", { a: 1 }), { ok: 1 });
  assert.equal(asked, 1);
  n = 0;
  answers.splice(0, 2, 401, 401);
  await assert.rejects(api.get("x"), (e: { kind: string }) => e.kind === "auth");
  assert.equal(asked, 2, "asked once per call, never in a loop");
  const no = createApi({ base: "https://example.com/b/", fetch: async () => new Response("{}", { status: 401, headers: { "content-type": "application/json" } }), onUnauthorized: async () => false });
  await assert.rejects(no.get("x"), (e: { kind: string }) => e.kind === "auth");
});

test("readMe: the answer; a stale auth session reloads; anything else falls back to ALL", async () => {
  const reloads: number[] = [];
  const loc = { reload: () => void reloads.push(1) };
  assert.deepEqual(await readMe({ get: async () => me(["*"]) }, loc), me(["*"]));
  assert.equal(await readMe({ get: async () => Promise.reject(Object.assign(new Error(), { kind: "auth", body: { reason: "expired" } })) }, loc), ALL);
  assert.equal(reloads.length, 1);
  assert.equal(await readMe({ get: async () => Promise.reject(Object.assign(new Error(), { kind: "auth", body: { error: "unauthorized" } })) }, loc), ALL);
  assert.equal(await readMe({ get: async () => Promise.reject(Object.assign(new Error(), { kind: "auth", body: null })) }, loc), ALL);
  assert.equal(await readMe({ get: async () => Promise.reject(Object.assign(new Error(), { kind: "notfound" })) }, loc), ALL);
  assert.equal(reloads.length, 1);
});

test("renew: no outside auth mode; refresh then retry; otherwise to the login page", async () => {
  const assigned: string[] = [];
  const location = { pathname: "/newsletter-admin/", search: "", hash: "#/campaigns", assign: (u: string) => void assigned.push(u) };
  const calls: [string, RequestInit][] = [];
  const ok = async (url: string, init: RequestInit) => (calls.push([url, init]), new Response(null, { status: 200 }));
  assert.equal(await renew(ALL, { fetch: ok, location }), false);
  assert.equal(calls.length, 0);
  assert.equal(await renew(me(["*"]), { fetch: ok, location }), true);
  assert.deepEqual(calls[0], ["/api/auth/refresh", { method: "POST", headers: { "x-ssg-auth": "1" }, credentials: "same-origin" }]);
  assert.equal(await renew(me(["*"]), { fetch: async () => new Response(null, { status: 401 }), location }), false);
  assert.equal(await renew(me(["*"]), { fetch: async () => Promise.reject(new TypeError("offline")), location }), false);
  assert.deepEqual(assigned, ["/auth/login?next=%2Fnewsletter-admin%2F%23%2Fcampaigns", "/auth/login?next=%2Fnewsletter-admin%2F%23%2Fcampaigns"]);
});
