// GO-181: a module declares a permission sensitive in its module.json
// ("sensitive": true); the declaration reaches auth through
// AUTH_KNOWN_PERMISSIONS (scripts/auth-permissions.mjs) and the registry, and
// counts beside the name rules of isSensitive — for service tokens and for the
// default role of Access auto-provisioning. The name rules alone are
// service-perms.test.ts's, unchanged.
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { defaultRole } from "../functions/api/auth/_access-provision.ts";
import { BUNDLED_SENSITIVE } from "../functions/api/auth/_bundled-sensitive.ts";
import { buildRegistry, registryOf } from "../functions/api/auth/_registry.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import { isSensitive, machineSensitive, tokenPermissions } from "../functions/api/auth/_service-perms.ts";
import { T0, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { account, claimsOf, secretFor, swap } from "./service.ts";

const DESIGNER = ["designer-ui:read", "designer-ui:settings", "designer-ui:advanced", "designer-ui:theme"];
const DECLARED: ReadonlySet<string> = new Set(["designer-ui:advanced", "designer-ui:theme"]);

// knownFor: the AUTH_KNOWN_PERMISSIONS value the script prints for these templates.
function knownFor(...templates: string[]): string {
  const root = new URL("../../", import.meta.url).pathname;
  const script = new URL("../scripts/auth-permissions.mjs", import.meta.url).pathname;
  const out = execFileSync(process.execPath, [script, ...templates.map((t) => `${root}${t}/module.json`)], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
  return (out.split("\n").find((l) => l.startsWith("AUTH_KNOWN_PERMISSIONS=")) as string).slice("AUTH_KNOWN_PERMISSIONS=".length);
}

test("isSensitive: a declared name is sensitive besides the name rules, which stay as they are", () => {
  for (const p of ["*", "auth:admin", "newsletter:erase", "designer-ui:*", "designer-ui:theme", "designer-ui:advanced"]) assert.ok(isSensitive(p, DECLARED), p);
  for (const p of ["designer-ui:read", "designer-ui:settings", "designer-ui:themes", "editor-ui:theme"]) assert.ok(!isSensitive(p, DECLARED), p);
  assert.ok(!isSensitive("designer-ui:theme"), "without the registry: the name rules alone");
  assert.equal(machineSensitive("designer-ui:theme", ["designer-ui:theme"], DECLARED), true, "a factor-free role does not free a declared name");
  assert.equal(machineSensitive("designer-ui:theme", ["designer-ui:theme"]), false);
});

test("tokenPermissions: a declared name reaches a token only through allowSensitive, also from * or a list", () => {
  assert.deepEqual(tokenPermissions(DESIGNER, null, [], DESIGNER, DECLARED), ["designer-ui:read", "designer-ui:settings"]);
  assert.deepEqual(tokenPermissions(["*"], ["designer-ui:theme", "designer-ui:read"], [], ["*"], DECLARED), ["designer-ui:read"]);
  assert.deepEqual(tokenPermissions(DESIGNER, null, ["designer-ui:theme"], DESIGNER, DECLARED), ["designer-ui:read", "designer-ui:settings", "designer-ui:theme"]);
  assert.deepEqual(tokenPermissions(DESIGNER, null, ["designer-ui:*"], DESIGNER, DECLARED), [...DESIGNER].sort());
  assert.deepEqual(tokenPermissions(DESIGNER, null, []), [...DESIGNER].sort(), "without `declared`: as before GO-181");
});

test("the registry: sensitive only for a literal true, from any declaration of the name; nothing else changes", () => {
  const r = buildRegistry([
    { id: "a", permissions: [{ name: "a:one", sensitive: true }, { name: "a:two", sensitive: "true" }, { name: "a:three", sensitive: 1 }, "a:four", { name: "a:five", sensitive: false }] },
    { id: "b", permissions: [{ name: "a:two", sensitive: true }, { name: "B:bad", sensitive: true }] },
  ]);
  assert.deepEqual([...r.sensitive].sort(), ["a:one", "a:two"], "a later declaration may only add protection");
  assert.equal(r.known.get("a:two")?.module, "a", "the first declaration still names the module");
  assert.deepEqual(Object.keys(r.known.get("a:one") ?? {}).sort(), ["description", "module", "name"], "the known entry's shape is unchanged");
  assert.equal(r.problems.length, 1);
  // GO-183: the templates' own sensitive names are always there (BUNDLED_SENSITIVE);
  // auth's own manifest still adds nothing — its names are sensitive by rule.
  assert.deepEqual([...registryOf({} as Env).sensitive].sort(), [...BUNDLED_SENSITIVE].sort(), "auth's own manifest declares nothing beyond the bundled names");
});

test("scripts/auth-permissions.mjs carries sensitive: true from module.json, and only where declared", () => {
  const value = JSON.parse(knownFor("designer-ui", "newsletter-ui"));
  const designer = value.find((m: { id: string }) => m.id === "designer-ui");
  assert.deepEqual(designer.permissions.filter((p: { sensitive?: boolean }) => p.sensitive).map((p: { name: string }) => p.name), ["designer-ui:advanced", "designer-ui:theme"]);
  assert.ok(value.flatMap((m: { permissions: object[] }) => m.permissions).every((p: { sensitive?: unknown }) => p.sensitive === undefined || p.sensitive === true));
  const reg = registryOf({ AUTH_KNOWN_PERMISSIONS: JSON.stringify(value) } as Env);
  // GO-183: editor-ui:publish is bundled-sensitive even when the value does not list editor-ui.
  assert.deepEqual([...reg.sensitive].sort(), ["designer-ui:advanced", "designer-ui:theme", "editor-ui:publish"]);
});

test("scripts/auth-permissions.mjs: a module without a panel or a title, and a value over 5 KB said so", () => {
  const dir = mkdtempSync(join(tmpdir(), "auth-perms-"));
  try {
    const bare = join(dir, "module.json");
    writeFileSync(bare, JSON.stringify({ id: "bare", kind: "pages-function", panel: null, permissions: [{ name: "bare:run", description: { en: "Run." } }] }));
    const script = new URL("../scripts/auth-permissions.mjs", import.meta.url).pathname;
    const r = spawnSync(process.execPath, [script, bare], { encoding: "utf8" });
    assert.equal(r.stdout.trim(), 'AUTH_KNOWN_PERMISSIONS=[{"id":"bare","permissions":[{"name":"bare:run","description":{"en":"Run."}}]}]');
    const root = new URL("../../", import.meta.url).pathname;
    const all = readdirSync(root).filter((d) => existsSync(join(root, d, "module.json"))).map((d) => join(root, d, "module.json"));
    const big = spawnSync(process.execPath, [script, "--all-languages", ...all], { encoding: "utf8" });
    assert.match(big.stderr, /over Cloudflare's 5 KB per variable/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("exchange: without the owner's allowSensitive a token never carries designer-ui:theme or :advanced; with it, it does", async () => {
  useClock();
  const env = makeEnv({ AUTH_KNOWN_PERMISSIONS: knownFor("designer-ui") } as Partial<Env>);
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, DESIGNER);
  const perms = async (json: Record<string, unknown>) => (await (await swap(env, `Bearer ${await secretFor(env, owner.browser, id, json)}`)).json()).permissions;
  assert.deepEqual(await perms({}), ["designer-ui:read", "designer-ui:settings"]);
  assert.deepEqual(await perms({ permissions: ["designer-ui:theme"] }), [], "asking for it in the list is not allowing it");
  const allowed = await (await swap(env, `Bearer ${await secretFor(env, owner.browser, id, { allowSensitive: ["designer-ui:theme", "designer-ui:advanced"] })}`)).json();
  assert.deepEqual(allowed.permissions, [...DESIGNER].sort());
  assert.deepEqual(claimsOf(allowed.accessToken).perms, allowed.permissions);
  // The same account, the registry not told: before GO-183 the name rules
  // alone applied and the token carried theme and advanced (the blind spot).
  // The templates' sensitive names are now bundled, so it fails closed.
  const blind = makeEnv();
  const owner2 = await signedIn(blind, "owner@example.com", "owner");
  const id2 = await account(blind, owner2.browser, DESIGNER);
  const plain = await (await swap(blind, `Bearer ${await secretFor(blind, owner2.browser, id2)}`)).json();
  assert.deepEqual(plain.permissions, ["designer-ui:read", "designer-ui:settings"]);
});

test("Access auto-provisioning refuses a default role holding a declared sensitive permission", async () => {
  useClock();
  const setup = { host: "h", aud: "a", linkByEmail: false, autoCreate: true, defaultRole: "designers" };
  const env = makeEnv({ AUTH_KNOWN_PERMISSIONS: knownFor("designer-ui") } as Partial<Env>);
  await ensureSchema(env);
  await createRole(env, { name: "designers", description: "", permissions: ["designer-ui:read", "designer-ui:theme"] }, T0);
  assert.deepEqual(await defaultRole(env, setup), { problem: 'AUTH_ACCESS_DEFAULT_ROLE "designers" holds designer-ui:theme; auto-created accounts must never be administrators' });
  const readers = makeEnv({ AUTH_KNOWN_PERMISSIONS: knownFor("designer-ui") } as Partial<Env>);
  await ensureSchema(readers);
  await createRole(readers, { name: "designers", description: "", permissions: ["designer-ui:read", "designer-ui:settings"] }, T0);
  assert.ok("roleId" in (await defaultRole(readers, setup)));
});
