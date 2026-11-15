// Operations: one implementation, two ways in (README of the round, decision
// 11). A REST route (routes.ts) and, later, an MCP tool call the same Op
// through perform(), and perform() is where the permission is checked — so
// no entry point can skip what another one enforces.
//
// Adding an operation:
//   1. write `export const myOp: Op<In, Out> = { name, permission: PERM.x, run }`
//      in its own file; `run` gets the verified caller, the env and the deps
//      (injected fetch/now — tests fake the repository host in-process);
//   2. throw OpError (or let a RepoError through) for a refusal — never
//      return an error object; errorResponse() turns both into JSON;
//   3. add a route in routes.ts (method, path, op, input) and the path to
//      openapi.yaml (test/openapi.test.ts keeps the two equal).

import type { Caller } from "./gate";
import { json, secure } from "./http";
import type { Env } from "./http";
import type { Permission } from "./permissions";
import { RepoError } from "../portable/repo-provider";
import type { FetchFn } from "../portable/repo-github-http";
import { can, forbidden } from "./session-verifier";
import type { AuthConfig } from "./session-verifier";

// Deps: what tests inject. fetch reaches the repository host (and the `auth`
// JWKS URL when one is configured); now() is in milliseconds.
export interface Deps {
  fetch?: FetchFn;
  now?: () => number;
}

export interface OpContext {
  env: Env;
  caller: Caller;
  auth: AuthConfig; // the validated EDITOR_AUTH_* set (login URL, issuer)
  deps: Deps;
  request: Request;
  url: URL;
}

export interface Op<I = unknown, O = unknown> {
  name: string; // the MCP tool name later; a stable id now
  permission: Permission;
  run(ctx: OpContext, input: I): Promise<O> | O;
}

// OpError: a refusal the caller may read — a snake_case code, the HTTP
// status, a detail we wrote ourselves, and extra fields for the body.
export class OpError extends Error {
  readonly code: string;
  readonly status: number;
  readonly detail?: string;
  readonly extra: Record<string, unknown>;
  constructor(code: string, status: number, detail?: string, extra: Record<string, unknown> = {}) {
    super(detail ? `${code}: ${detail}` : code);
    this.name = "OpError";
    this.code = code;
    this.status = status;
    this.detail = detail;
    this.extra = extra;
  }
}

// perform checks the operation's permission, then runs it.
export async function perform<I, O>(op: Op<I, O>, ctx: OpContext, input: I): Promise<O> {
  if (!can(ctx.caller.perms, op.permission)) throw new OpError("forbidden", 403, undefined, { permission: op.permission });
  return op.run(ctx, input);
}

// errorResponse turns a thrown error into the answer. A missing permission is
// the verifier's own forbidden() (CONTRACT.md); anything unexpected is logged
// and answered with a bare 500 — its text might carry request data.
export function errorResponse(err: unknown, where: string): Response {
  if (err instanceof OpError) {
    if (err.code === "forbidden") return secure(forbidden(String(err.extra.permission)));
    return json({ error: err.code, ...(err.detail ? { detail: err.detail } : {}), ...err.extra }, err.status);
  }
  if (err instanceof RepoError) {
    const body: Record<string, unknown> = { error: err.code };
    if (err.detail) body.detail = err.detail;
    if (err.conflicts) body.conflicts = err.conflicts;
    if (err.retryAt) body.retryAt = err.retryAt;
    return json(body, err.status, err.code === "rate_limited" ? { "retry-after": retryAfter(err.retryAt) } : {});
  }
  console.error("editor-ui:", where, err);
  return json({ error: "internal_error" }, 500);
}

// retryAfter: seconds until the provider's limit resets (at least 1), or 60
// when it did not say.
export function retryAfter(retryAt: string | undefined, now = Date.now()): string {
  const at = Date.parse(retryAt ?? "");
  return Number.isFinite(at) ? String(Math.max(1, Math.ceil((at - now) / 1000))) : "60";
}
