// Schema version 2 (GO-133): the views and triggers in schema.sql are the
// runtime's, word for word; the built-in roles exist once and name only
// permissions the bundled manifests declare; an upgraded database gets its
// owner.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { BUILT_IN_ROLES, RBAC_MIGRATION } from "../functions/api/auth/_rbac-schema.ts";
import { ensureSchema, migrate } from "../functions/api/auth/_schema.ts";
import { fakeD1 } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";

const norm = (s: string) => s.replace(/\s+/g, " ").replace(/\( /g, "(").replace(/ \)/g, ")").trim();
const logic = (d1: FakeD1) =>
  d1.sqlite
    .prepare("SELECT name, sql FROM sqlite_master WHERE type IN ('trigger', 'view', 'index') AND sql IS NOT NULL ORDER BY name")
    .all()
    .map((r) => `${r.name}: ${norm(String(r.sql))}`);

test("schema.sql: the v2 views, triggers and indexes are the runtime's, statement for statement", async () => {
  const fromCode = fakeD1();
  await ensureSchema({ AUTH_DB: fromCode } as never);
  const fromFile = fakeD1();
  fromFile.sqlite.exec(readFileSync(new URL("../schema.sql", import.meta.url), "utf8"));
  assert.deepEqual(logic(fromFile), logic(fromCode));
  assert.equal(logic(fromCode).filter((l) => l.startsWith("auth_owner_")).length, 12);
});

test("built-in roles: seeded once with their permissions; a hand-applied schema.sql gets them on first use", async () => {
  const d1 = fakeD1();
  d1.sqlite.exec(readFileSync(new URL("../schema.sql", import.meta.url), "utf8"));
  assert.equal((d1.sqlite.prepare("SELECT COUNT(*) AS n FROM roles").get() as { n: number }).n, 0);
  await ensureSchema({ AUTH_DB: d1 } as never);
  await migrate(d1 as never, [RBAC_MIGRATION]);
  const rows = d1.sqlite.prepare("SELECT r.name, r.built_in, group_concat(p.permission) AS perms FROM roles r JOIN role_permissions p ON p.role_id = r.id GROUP BY r.id ORDER BY r.name").all();
  assert.deepEqual(
    rows.map((r) => ({ ...r })),
    [...BUILT_IN_ROLES].sort((a, b) => (a.name < b.name ? -1 : 1)).map((r) => ({ name: r.name, built_in: 1, perms: r.permissions.join(",") })),
  );
  assert.equal((d1.sqlite.prepare("SELECT version FROM schema_meta").get() as { version: number }).version, 2);
});

test("built-in roles name only permissions the bundled manifests declare", () => {
  const declared = new Set<string>();
  for (const m of ["newsletter-ui", "newsletter", "comments", "auth"]) {
    const manifest = JSON.parse(readFileSync(new URL(`../../${m}/module.json`, import.meta.url), "utf8"));
    for (const p of manifest.permissions) declared.add(p.name);
  }
  for (const r of BUILT_IN_ROLES) {
    for (const p of r.permissions) assert.ok(p === "*" || declared.has(p), `${r.name}: ${p} is declared by no manifest`);
  }
  assert.deepEqual(
    BUILT_IN_ROLES.map((r) => r.name),
    ["owner", "newsletter-editor", "newsletter-viewer", "comments-moderator"],
  );
  const editor = BUILT_IN_ROLES[1].permissions;
  assert.ok(editor.includes("newsletter:send") && !editor.includes("newsletter:erase"), "an editor sends but does not erase");
});

test("upgrade from version 1: the oldest active account becomes the owner, once", async () => {
  const d1 = fakeD1();
  await migrate(d1 as never, []);
  const add = (id: string, at: string, status = "active") =>
    d1.sqlite.prepare("INSERT INTO users (id, email, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)").run(id, `${id}@example.com`, status, at, at);
  add("blocked-first", "2026-01-01", "blocked");
  add("second", "2026-01-03");
  add("first", "2026-01-02");
  await migrate(d1 as never, [RBAC_MIGRATION]);
  const owners = () => d1.sqlite.prepare("SELECT user_id FROM grants").all().map((r) => r.user_id);
  assert.deepEqual(owners(), ["first"]);
  d1.sqlite.exec("UPDATE schema_meta SET version = 1");
  await migrate(d1 as never, [RBAC_MIGRATION]);
  assert.deepEqual(owners(), ["first"], "an owner exists: nobody else is made one");
  assert.deepEqual(d1.sqlite.prepare("SELECT id FROM active_owners").all().map((r) => r.id), ["first"]);
});
