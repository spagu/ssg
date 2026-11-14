// Accounts and logins for the route tests.
import { hashPassword } from "../functions/api/auth/_password.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { createUser } from "../functions/api/auth/_users.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { Browser, PASSWORD, T0, call } from "./fakes.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

export async function seedUser(env: Env, email = "ada@example.com", password = PASSWORD, iterations = 10_000): Promise<string> {
  await ensureSchema(env);
  return (await createUser(env, { email, name: "Ada", passwordHash: await hashPassword(password, iterations) }, T0)) as string;
}

// loggedIn seeds an account and logs a fresh browser into it.
export async function loggedIn(env: Env, email = "ada@example.com"): Promise<{ browser: Browser; id: string; res: Response }> {
  const id = await seedUser(env, email);
  const browser = new Browser();
  const res = await call(login, env, "/api/auth/login", { json: { email, password: PASSWORD }, browser });
  if (res.status !== 200) throw new Error(`login failed: ${res.status}`);
  return { browser, id, res };
}

// Roles and grants for the permission tests (GO-133).
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";

export function roleId(env: Env & { AUTH_DB: { sqlite: { prepare(sql: string): { get(...a: unknown[]): unknown } } } }, name: string): string {
  return (env.AUTH_DB.sqlite.prepare("SELECT id FROM roles WHERE name = ?").get(name) as { id: string }).id;
}

let roles = 0;
// grantPerms creates a role holding exactly `perms` and grants it to the user.
export async function grantPerms(env: Env, userId: string, perms: string[]): Promise<string> {
  const role = (await createRole(env, { name: `test-role-${++roles}`, description: "", permissions: perms }, T0)) as Role;
  await addGrant(env, role.id, { userId }, "test", T0);
  return role.id;
}

// signedIn seeds an account holding `perms` (through one role, or the
// built-in owner role for "owner") and logs a fresh browser into it.
export async function signedIn(env: Env, email: string, perms: string[] | "owner"): Promise<{ browser: Browser; id: string }> {
  const id = await seedUser(env, email);
  if (perms === "owner") await addGrant(env, roleId(env as never, "owner"), { userId: id }, "test", T0);
  else await grantPerms(env, id, perms);
  const browser = new Browser();
  const res = await call(login, env, "/api/auth/login", { json: { email, password: PASSWORD }, browser });
  if (res.status !== 200) throw new Error(`login failed: ${res.status}`);
  return { browser, id };
}
