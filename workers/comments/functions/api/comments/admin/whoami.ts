// Cloudflare Pages Function: who the moderation panel is talking to (GO-136).
//   GET /api/comments/admin/whoami
//     200 {mode: "access" | "auth" | "password", user: {id, email} | null,
//          permissions: [...]}  — the declared permissions the caller holds
//     refusals: exactly requireAdmin's (Access, password, auth); in auth mode a
//     401 also carries {mode: "auth", login}: where the panel sends a browser
//     without a session (COMMENTS_AUTH_LOGIN_URL with next = the panel)
//
// The panel asks here first and draws itself for the mode: a password field,
// or the signed-in account and only the actions it may perform. It needs no
// permission — a session without comments:read must still learn that it lacks
// it — and no database. No existing response changes.

import { json, requireAdmin } from "../_lib";
import type { Env } from "../_lib";
import { permissionsOf } from "../_admin-auth";
import { MODULE } from "../_module";
import { configFrom, loginRedirect } from "../_session-verifier";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const admin = await requireAdmin(request, env);
  if (!(admin instanceof Response)) return json({ mode: admin.mode, user: admin.user, permissions: permissionsOf(admin) });
  const auth = env.COMMENTS_ACCESS_TEAM && env.COMMENTS_ACCESS_AUD ? null : configFrom(env, "COMMENTS");
  if (!auth || admin.status !== 401) return admin;
  // The login URL as the verifier builds it for a browser opening the panel —
  // from configuration only, so the panel never composes one from input.
  const panel = new Request(new URL(MODULE.panel, request.url), { headers: { accept: "text/html" } });
  const login = loginRedirect(panel, auth).headers.get("location");
  const body = { ...((await admin.json()) as Record<string, unknown>), mode: "auth", login };
  return new Response(JSON.stringify(body), { status: 401, headers: admin.headers });
};
