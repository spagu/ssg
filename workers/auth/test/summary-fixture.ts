// A database where every part of the auth summary shows (GO-138): an old
// signing key, open invitations, an account whose role needs a second factor
// it lacks and a broken mail setting. Shared by summary-sample.ts (workers/modules.test.ts) and
// admin-summary.test.ts.
import { onRequestGet as summary } from "../functions/api/auth/module/summary.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createInvitation } from "../functions/api/auth/_invites.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { T0, call, useClock } from "./fakes.ts";
import { mfaEnv, signIn } from "./mfa.ts";
import { seedUser, signedIn } from "./seed.ts";

export async function summaryAnswer(over: Record<string, string> = {}): Promise<Response> {
  useClock();
  const env = mfaEnv({ AUTH_MFA_ROLES: '{"panel-admin": false}', AUTH_RESEND_API_KEY: "re_x", ...over });
  const admin = await seedUser(env, "admin@example.com");
  const role = (await createRole(env, { name: "panel-admin", description: "", permissions: ["auth:admin"] }, T0)) as Role;
  await addGrant(env, role.id, { userId: admin }, "t", T0);
  const { browser } = await signIn(env, "admin@example.com");
  await signedIn(env, "sender@example.com", ["newsletter:send"]);
  await createInvitation(env, { email: "new@example.com", name: "", roleIds: [], groupIds: [], lang: "en", invitedBy: admin }, T0);
  env.AUTH_DB.sqlite.prepare("UPDATE signing_keys SET first_used_at = ?").run(new Date(T0 - 400 * 86_400_000).toISOString());
  return call(summary, env, "/api/auth/module/summary", { browser });
}
