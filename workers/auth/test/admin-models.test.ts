// GO-138: the admin panel's logic, as pure functions: what an account may
// see and press, the step-up flow, the permission registry, the access
// matrix, the filters, the Status judgements and the audit vocabulary.
import { test } from "node:test";
import assert from "node:assert/strict";
import { allows, can, home, navFor, shown, viewLacks } from "../public/auth/assets/admin/js/models/access.js";
import { MARK, arrival, initial, next, stepUpHref, withMark } from "../public/auth/assets/admin/js/models/stepup.js";
import { coveredByWildcard, describe, isWildcard, modulesOf, prefixesOf, toggle, unknownOf, validPermission, wildcardsIn } from "../public/auth/assets/admin/js/models/perms.js";
import { cellOf, filterRows, rowsOf, tally } from "../public/auth/assets/admin/js/models/matrix.js";
import { auditFilter, formData, lastDays, pick, toApi, userFilter } from "../public/auth/assets/admin/js/models/filters.js";
import { keyAdvice, staleness, warnings } from "../public/auth/assets/admin/js/models/status.js";
import { EVENT_KINDS, detailPairs } from "../public/auth/assets/admin/js/models/events.js";
import { can as serverCan } from "../functions/api/auth/_can.ts";
import { readFileSync } from "node:fs";

test("access: can() is the worker's own rule, case by case", () => {
  const cases: [string[], string][] = [[["*"], "auth:admin"], [["auth:*"], "auth:invite"], [["auth:admin"], "auth:admin"], [["auth:invite"], "auth:admin"], [["newsletter:*"], "auth:admin"], [[], "x:y"], [["auth:admin"], "Auth:admin"], [["auth:admin"], "auth:admin "]];
  for (const [perms, need] of cases) assert.equal(can(perms, need), serverCan(perms, need), `${perms} → ${need}`);
  assert.equal(can(null, "auth:admin"), false);
  // "Holds *?" — the owner-only buttons; the server's can() refuses wildcard questions.
  assert.deepEqual([can(["*"], "*"), can(["auth:*"], "*"), can(["auth:*"], "auth:*"), can(["auth:admin"], "auth:*")], [true, false, true, false]);
});

test("access: views, buttons and the navigation follow the token's permissions", () => {
  const admin = { perms: ["auth:admin"], access: "absent" };
  const inviter = { perms: ["auth:invite"], access: "absent" };
  const owner = { perms: ["*"], access: "enabled" };
  assert.deepEqual(navFor(admin), ["people", "groups", "roles", "matrix", "sessions", "services", "audit", "status"]);
  assert.deepEqual(navFor(inviter), ["invitations"]);
  assert.ok(navFor(owner).includes("mappings"));
  assert.ok(navFor({ perms: ["*"], access: "misconfigured" }).includes("mappings"));
  assert.equal(home(admin), "people");
  assert.equal(home(inviter), "invitations");
  assert.equal(home({ perms: [], access: "absent" }), "invitations");
  assert.equal(viewLacks(inviter, "people"), "auth:admin");
  assert.equal(viewLacks(inviter, "notFound"), null);
  assert.equal(viewLacks(inviter, "nonsense"), null);
  assert.deepEqual([shown(admin, "mappings"), shown(admin, "people")], [false, true]);
  assert.deepEqual([allows(admin, "resetFactor"), allows(owner, "resetFactor"), allows(admin, "chooseRoles"), allows(inviter, "invite"), allows(admin, "auth:admin")], [false, true, true, true, true]);
});

test("step-up: the states, the way out and the way back — the action is never replayed", () => {
  assert.equal(next("idle", "refused"), "needed");
  assert.equal(next("needed", "go"), "leaving");
  assert.equal(next("needed", "cancel"), "idle");
  assert.equal(next("needed", "refused"), "needed");
  assert.equal(next("returned", "dismiss"), "idle");
  assert.equal(next("returned", "refused"), "needed");
  assert.equal(next("leaving", "refused"), "leaving", "an unexpected event changes nothing");
  assert.equal(next("nonsense", "go"), "nonsense");
  assert.equal(withMark("#/people/x"), `#/people/x?${MARK}=1`);
  assert.equal(withMark("#/people?q=a"), "#/people?q=a&stepped=1");
  assert.equal(withMark(""), "#/?stepped=1");
  assert.equal(withMark("#main"), "#/?stepped=1");
  assert.equal(stepUpHref("/auth/mfa/step-up", "/auth/admin/", "#/people/x"), "/auth/mfa/step-up?next=%2Fauth%2Fadmin%2F%23%2Fpeople%2Fx%3Fstepped%3D1");
  assert.equal(stepUpHref("/s?a=1", "/p", ""), "/s?a=1&next=%2Fp%23%2F%3Fstepped%3D1");
  assert.deepEqual(arrival("#/people/x?q=a&stepped=1"), { returned: true, hash: "#/people/x?q=a" });
  assert.deepEqual(arrival("#/people/x?stepped=1"), { returned: true, hash: "#/people/x" });
  assert.deepEqual(arrival("#/people?q=a"), { returned: false, hash: "#/people?q=a" });
  assert.deepEqual(arrival("#/people"), { returned: false, hash: "#/people" });
  assert.deepEqual(arrival(undefined as never), { returned: false, hash: "" });
  assert.deepEqual([initial("#/x?stepped=1"), initial("#/x")], ["returned", "idle"]);
});

const REGISTRY = [
  { name: "auth:admin", module: "auth", description: { en: "Administer", pl: "Administrować" }, known: true },
  { name: "auth:invite", module: "auth", description: null, known: true },
  { name: "newsletter:send", module: "newsletter-ui", description: { en: "Send" }, known: true },
  { name: "newsletter:read", module: "newsletter-ui", description: { en: "Read" }, known: true },
  { name: "private:thing", module: null, description: null, known: false },
];

test("perms: grouped by module in the viewer's language; wildcards, unknown names, toggling", () => {
  const mods = modulesOf(REGISTRY, "pl");
  assert.deepEqual(mods.map((m: { id: string }) => m.id), ["auth", "newsletter-ui"]);
  assert.deepEqual(mods[0].permissions, [{ name: "auth:admin", description: "Administrować" }, { name: "auth:invite", description: null }]);
  assert.deepEqual(mods[1].permissions.map((p: { name: string }) => p.name), ["newsletter:read", "newsletter:send"]);
  assert.equal(describe(REGISTRY[2], "de"), "Send");
  assert.equal(describe({ description: { pl: "x" } }, "de"), null);
  assert.equal(describe(null, "en"), null);
  assert.deepEqual(prefixesOf(mods[1]), ["newsletter"]);
  assert.deepEqual(unknownOf(["*", "auth:*", "newsletter:send", "private:thing", "ghost:*"], mods), ["private:thing", "ghost:*"]);
  assert.deepEqual(wildcardsIn(["*", "auth:admin", "x:*"]), ["*", "x:*"]);
  assert.deepEqual([isWildcard("*"), isWildcard("a:*"), isWildcard("a:b")], [true, true, false]);
  assert.deepEqual(toggle(["b:c", "a:b"], "a:a", true), ["a:a", "a:b", "b:c"]);
  assert.deepEqual(toggle(["a:b"], "a:b", false), []);
  assert.deepEqual([coveredByWildcard(["auth:*"], "auth:admin"), coveredByWildcard(["auth:admin"], "auth:admin"), coveredByWildcard([], "auth:admin")], [true, false, false]);
  for (const ok of ["*", "a:*", "a-b:c-d"]) assert.ok(validPermission(ok), ok);
  for (const bad of ["*:x", "A:b", "a", "a:b:c", "", null, "a:" + "b".repeat(70)]) assert.ok(!validPermission(bad), String(bad));
});

test("matrix: none / some / all per module, the tally, the filters", () => {
  const mods = modulesOf(REGISTRY, "en");
  const users = [
    { id: "1", permissions: ["*"] },
    { id: "2", permissions: ["newsletter:read"] },
    { id: "3", permissions: ["auth:*", "newsletter:read", "newsletter:send"] },
    { id: "4", permissions: [] },
  ];
  assert.deepEqual(cellOf(["newsletter:read"], mods[1]), { module: "newsletter-ui", held: 1, total: 2, level: "some" });
  const rows = rowsOf(users, mods);
  assert.deepEqual(rows.map((r: { cells: { level: string }[] }) => r.cells.map((c) => c.level)), [["all", "all"], ["none", "some"], ["all", "all"], ["none", "none"]]);
  assert.deepEqual(tally(rows, mods), [{ module: "auth", all: 2, some: 0, none: 2 }, { module: "newsletter-ui", all: 2, some: 1, none: 1 }]);
  const ids = (r: { user: { id: string } }[]) => r.map((x) => x.user.id);
  assert.deepEqual(ids(filterRows(rows, mods, { module: "newsletter-ui", level: "any" })), ["1", "2", "3"]);
  assert.deepEqual(ids(filterRows(rows, mods, { module: "auth", level: "none" })), ["2", "4"]);
  assert.equal(filterRows(rows, mods, { module: "ghost", level: "all" }).length, 4);
  assert.equal(filterRows(rows, mods, { module: "auth" }).length, 4);
  assert.equal(filterRows(rows, mods).length, 4);
});

test("filters: from the address bar and forms, dates checked, the API's exclusive end", () => {
  assert.deepEqual(pick({ a: " x ", b: "", c: 3 }, ["a", "b", "c"]), { a: "x" });
  assert.deepEqual(userFilter({ q: "ann", status: "gone", roleId: "r" }), { q: "ann", roleId: "r" });
  assert.deepEqual(userFilter({ status: "blocked" }), { status: "blocked" });
  assert.deepEqual(auditFilter({ kind: "signin", from: "2026-11-01", to: "soon", user: "u" }), { kind: "signin", from: "2026-11-01", user: "u" });
  assert.deepEqual(toApi({ from: "2026-11-01", to: "2026-11-30" }), { from: "2026-11-01", to: "2026-12-01" });
  assert.deepEqual(toApi({ kind: "x" }), { kind: "x" });
  assert.deepEqual(lastDays(7, Date.parse("2026-11-14T12:00:00Z")), { from: "2026-11-08", to: "2026-11-14" });
  assert.ok(lastDays(1).from);
  const data = new Map([["q", "  bob "], ["status", ""]]);
  assert.deepEqual(formData({ get: (k: string) => data.get(k) ?? null }, ["q", "status", "roleId"]), { q: "bob" });
});

test("status: key advice, the staleness split, warnings most serious first", () => {
  assert.deepEqual([keyAdvice({ ageDays: null }), keyAdvice({}), keyAdvice({ ageDays: 400, rotate: true }), keyAdvice({ ageDays: 3, rotate: false })], ["unknown", "unknown", "rotate", "ok"]);
  assert.deepEqual(staleness(630), { minutes: 10, seconds: 30 });
  const fine = { keys: [{ current: true, rotate: false }], mfa: { state: "off", policy: "default", accountsMissingFactor: 3 }, mail: { state: "none" }, access: "absent", registryProblems: [] };
  assert.deepEqual(warnings(fine), []);
  const bad = { keys: [{ current: true, rotate: true }], mfa: { state: "misconfigured", policy: "invalid", accountsMissingFactor: 2 }, mail: { state: "broken" }, access: "misconfigured", registryProblems: ["x"] };
  assert.deepEqual(warnings(bad), ["keyOld", "mfaMisconfigured", "mfaPolicyInvalid", "missingFactor", "mailBroken", "accessMisconfigured", "registry"]);
});

test("events: every kind the worker writes is in the filter; details read as pairs", () => {
  const source = readFileSync(new URL("../functions/api/auth/_audit.ts", import.meta.url), "utf8");
  const declared = [...source.slice(source.indexOf("export type EventKind"), source.indexOf("export interface AuthEvent")).replace(/\/\/.*$/gm, "").matchAll(/"([a-z_]+)"/g)].map((m) => m[1]).sort();
  assert.deepEqual([...EVENT_KINDS].sort(), declared);
  assert.deepEqual(detailPairs({ by: "x", role: "reader", n: 2, empty: "", none: null, obj: { a: 1 } }), [["n", "2"], ["obj", '{"a":1}'], ["role", "reader"]]);
  assert.deepEqual([detailPairs(null), detailPairs("x")], [[], []]);
});
