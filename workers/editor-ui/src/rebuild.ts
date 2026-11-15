// After an MDDB write: the site is built from MDDB, so it shows the change
// only after its next build. EDITOR_REBUILD_URL (optional) starts one — e.g.
// the republish-trigger worker's POST /api/republish on the same site.
//
// The URL must be on the editor's own origin (a path such as
// "/api/republish", or an absolute URL with the same scheme, host and port):
// the call never leaves the site. It carries a credential of its own, the
// secret EDITOR_REBUILD_KEY, as `Authorization: Bearer` (republish-trigger's
// REPUBLISH_KEY, or a service token of `auth` holding republish-trigger:run).
// The caller's own cookie or token is never forwarded: it was issued for the
// editor, passing it on would let any endpoint behind that URL act as the
// caller, and republish-trigger reads no session cookie anyway. Without the
// key nothing is called — a rebuild URL must not be open to whoever publishes.

import type { MddbEnv } from "./mddb-config";
import type { OpContext } from "./ops";

export type RebuildEnv = MddbEnv & { EDITOR_REBUILD_URL?: string; EDITOR_REBUILD_KEY?: string };
export interface Rebuild { triggered: boolean; status?: number; reason?: "not_configured" | "bad_url" | "refused" | "unreachable" }

export const SAVED_NOTICE = "saved in MDDB; the site shows it after the next build";
export const STARTED_NOTICE = "saved in MDDB; a rebuild of the site was started";
export const TIMEOUT_MS = 10_000;

// rebuildUrl: the URL on the request's origin, or null.
export function rebuildUrl(raw: string, origin: URL): URL | null {
  try {
    const u = new URL(raw, origin);
    return u.origin === origin.origin && !u.username && !u.password ? u : null;
  } catch {
    return null;
  }
}

// triggerRebuild: what happened, and the sentence the answer carries.
export async function triggerRebuild(ctx: OpContext): Promise<{ rebuild: Rebuild; notice: string }> {
  const env = ctx.env as RebuildEnv;
  const raw = (env.EDITOR_REBUILD_URL ?? "").trim();
  const key = (env.EDITOR_REBUILD_KEY ?? "").trim();
  const not = (reason: Rebuild["reason"], status?: number) => ({ rebuild: { triggered: false, ...(status ? { status } : {}), reason }, notice: SAVED_NOTICE });
  if (!raw || !key) return not("not_configured");
  const url = rebuildUrl(raw, ctx.url);
  if (!url) return not("bad_url");
  const doFetch = ctx.deps.fetch ?? ((u: string, init?: RequestInit) => fetch(u, init));
  try {
    const res = await doFetch(url.href, { method: "POST", headers: { authorization: `Bearer ${key}` }, redirect: "manual", signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (res.status >= 200 && res.status < 300) return { rebuild: { triggered: true, status: res.status }, notice: STARTED_NOTICE };
    return not("refused", res.status);
  } catch {
    return not("unreachable");
  }
}
