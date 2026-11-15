// What makes this template designer-ui: its id, its variable prefix, its
// default base path and the environment it is given. The shared modules in
// portable/ (copied byte for byte from workers/editor-ui/portable/, GO-185)
// read these names from here; what every answer carries is
// portable/http-kit.ts.
//
// A standalone Worker (`wrangler deploy` from workers/designer-ui). It has no
// database: the history of changes is the repository's history (README of the
// round, "Konfiguracja"). Everything about the site is read from the branch.

import { normalizeBase } from "../portable/http-kit";

export interface Env {
  ASSETS?: Fetcher; // the panel files in public/, served behind the gate
  RATE_LIMITER?: RateLimit; // per-IP request volume, optional
  DESIGNER_BASE?: string; // path prefix, default /designer
  DESIGNER_PLAN?: string; // "free" (default) or "paid": the publication limits (portable/plan.ts)
  // The repository (portable/repo-config.ts reads them).
  DESIGNER_GIT_REPO?: string; // "github.com/<owner>/<repo>"
  DESIGNER_GIT_BRANCH?: string; // empty = the repository's default branch
  DESIGNER_GIT_TOKEN?: string; // a secret, never a [vars] entry
  // `auth` sessions, read by the portable verifier (src/session-verifier.ts).
  // Required: an incomplete or missing set answers 503 everywhere.
  DESIGNER_AUTH_ISSUER?: string;
  DESIGNER_AUTH_AUDIENCE?: string;
  DESIGNER_AUTH_PUBLIC_KEY?: string;
  DESIGNER_AUTH_JWKS_URL?: string;
  DESIGNER_AUTH_LOGIN_URL?: string;
}

// MODULE_ID: module.json's id — the permission namespace, the log and
// rate-limit prefix, the MCP server's name. PREFIX: DESIGNER_AUTH_*, DESIGNER_GIT_*.
export const MODULE_ID = "designer-ui";
export const PREFIX = "DESIGNER";
export const DEFAULT_BASE = "/designer";

// basePath: DESIGNER_BASE normalised, or null when it is not a plain path.
export const basePath = (env: Env): string | null => normalizeBase(env.DESIGNER_BASE, DEFAULT_BASE);
