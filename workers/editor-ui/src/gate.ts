// The gate: who is asking. editor-ui has ONE sign-in mode — `auth` (README of
// the round, decision 1; docs/MODULE-REQUIREMENTS.md lets a module that
// publishes on someone's behalf offer `auth` alone). Every publication needs
// an author and a permission per action; a shared password gives neither, so
// there is none, and no Access-only mode either.
//
//   - EDITOR_AUTH_* missing or incomplete → 503 auth_not_configured on every
//     path, the panel's files included. Never open, never a weaker mode.
//   - A person: the `auth` session cookie. A script: `Authorization: Bearer`
//     with a service token. The portable verifier enforces the channel per
//     kind (`wrong_kind`); this file never works around it.
//   - A refusal is the verifier's own answer: denied() (a page is sent to the
//     login URL with a safe `next`, an API call gets 401 JSON).
//   - CSRF: a state-changing request authenticated by the cookie must show it
//     comes from this origin (csrfRefusal). A bearer request cannot be forged
//     by another page, so it needs no such check.
//
// What a caller may do is not decided here: each operation checks its own
// permission (ops.ts), so the REST routes and the MCP endpoint refuse alike.

import { PREFIX, json, secure } from "./http";
import type { Env } from "./http";
import { configFrom, denied, verifySession } from "./session-verifier";
import type { AuthConfig, Deps } from "./session-verifier";

// Caller: the verified identity. kind "user" came by cookie, "service" by bearer.
export interface Caller {
  kind: "user" | "service";
  user: { id: string; email: string };
  perms: string[];
  sessionId: string;
  expiresAt: string;
}

export type GateResult = { ok: true; caller: Caller; config: AuthConfig } | { ok: false; response: Response };

// NOT_CONFIGURED: what an operator reads in the 503 when no EDITOR_AUTH_* is set.
export const NOT_CONFIGURED =
  "editor-ui accepts accounts of the auth worker only: set EDITOR_AUTH_ISSUER, EDITOR_AUTH_LOGIN_URL and " +
  "EDITOR_AUTH_PUBLIC_KEY (or EDITOR_AUTH_JWKS_URL)";

// authConfig: the complete EDITOR_AUTH_* set, or what is wrong with it (names
// variables, never values).
export function authConfig(env: Env): { ok: true; config: AuthConfig } | { ok: false; detail: string } {
  const config = configFrom(env, PREFIX);
  if (!config) return { ok: false, detail: NOT_CONFIGURED };
  if (config.configProblem) return { ok: false, detail: config.configProblem };
  return { ok: true, config };
}

// authenticate verifies the request's credential, or returns the answer to send.
export async function authenticate(request: Request, env: Env, deps: Deps = {}): Promise<GateResult> {
  const auth = authConfig(env);
  if (!auth.ok) return { ok: false, response: json({ error: "auth_not_configured", detail: auth.detail }, 503) };
  const v = await verifySession(request, auth.config, deps);
  if (!v.ok) return { ok: false, response: secure(denied(request, auth.config, v.reason)) };
  return { ok: true, config: auth.config, caller: { kind: v.kind, user: v.user, perms: v.perms, sessionId: v.sessionId, expiresAt: v.expiresAt } };
}

// SAFE_METHODS change nothing; every other method is state-changing.
export const SAFE_METHODS = new Set(["GET", "HEAD"]);

// csrfRefusal: null when the request may go on, else 403 cross_origin. For a
// cookie session a state-changing request must carry evidence of this origin —
// Origin equal to it or Sec-Fetch-Site "same-origin" — and nothing that says
// otherwise (a sibling subdomain is "same-site", and may be someone else's).
// No evidence at all is refused: browsers send at least one of the two.
export function csrfRefusal(request: Request, caller: Caller): Response | null {
  if (SAFE_METHODS.has(request.method) || caller.kind === "service") return null;
  const origin = request.headers.get("origin");
  const site = request.headers.get("sec-fetch-site");
  const own = new URL(request.url).origin;
  const foreign = (origin !== null && origin !== own) || (site !== null && site !== "same-origin");
  if (!foreign && (origin !== null || site !== null)) return null;
  return json({ error: "cross_origin", detail: "a state-changing request must come from this site's own pages" }, 403);
}
