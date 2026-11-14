// Who may call POST /api/push/notify. A leading underscore keeps this file out
// of the Pages route table.
//
// Two ways, both for machines (CI after a deploy, a cron):
//   1. the shared PUSH_KEY as `Authorization: Bearer <key>` — as always;
//   2. when PUSH_AUTH_* is set (GO-141): a service access token from the
//      `auth` worker (POST /api/auth/token) whose permissions include
//      `web-push:notify`, checked locally by the portable verifier.
// Without PUSH_AUTH_* nothing changes: the answers are byte for byte those of
// 1.9.8 (test/compat-notify.test.ts). With both set, both are accepted — the
// key opens this one action and nothing else, so accepting it beside `auth`
// bypasses no permission. A half-set PUSH_AUTH_* is a 503, never a silent
// fall-back to the key alone.
//
// A person's session cookie is never read here: only the bearer header
// reaches the verifier, and the verifier takes service tokens only on that
// channel. A "notify now" button for a signed-in person would be a separate,
// CSRF-checked route; it is deliberately not part of this endpoint.

import { json, timingSafeEqual } from "./_lib";
import type { Env } from "./_lib";
import { can, configFrom, denied, forbidden, verifySession } from "./_session-verifier";

export const NOTIFY_PERMISSION = "web-push:notify";

// authorizeNotify returns null when the caller may notify, else the answer to send.
export async function authorizeNotify(request: Request, env: Env): Promise<Response | null> {
  const auth = configFrom(env, "PUSH");
  const header = request.headers.get("authorization") || "";
  const token = header.startsWith("Bearer ") ? header.slice(7) : "";
  if (!auth) {
    if (!env.PUSH_KEY) return json({ error: "not_configured" }, 503);
    return timingSafeEqual(token, env.PUSH_KEY) ? null : json({ error: "unauthorized" }, 401);
  }
  // Only the bearer header, on a POST: no cookie can be read, and a failure is
  // always JSON (a machine is never redirected to a login page).
  const bare = new Request(request.url, { method: "POST", headers: { authorization: header } });
  if (auth.configProblem) return denied(bare, auth, "not_configured");
  if (env.PUSH_KEY && timingSafeEqual(token, env.PUSH_KEY)) return null;
  const v = await verifySession(bare, auth);
  if (!v.ok) return denied(bare, auth, v.reason);
  return can(v.perms, NOTIFY_PERMISSION) ? null : forbidden(NOTIFY_PERMISSION);
}
