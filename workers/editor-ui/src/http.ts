// What makes this template editor-ui: its id, its variable prefix, its
// default base path and the environment it is given. The shared modules in
// portable/ (gate, operations, pipeline, repository, MCP endpoint) read these
// names from here, so they stay byte-identical across the panel workers
// (GO-185); what every answer carries is portable/http-kit.ts.
//
// A standalone Worker (`wrangler deploy` from workers/editor-ui). It has no
// database: the history of changes is the repository's history, drafts live
// in the browser (README of the round, "Konfiguracja").

import { normalizeBase } from "../portable/http-kit";

export interface Env {
  ASSETS?: Fetcher; // the panel files in public/, served behind the gate
  RATE_LIMITER?: RateLimit; // per-IP request volume, optional
  EDITOR_BASE?: string; // path prefix, default /editor
  EDITOR_PLAN?: string; // "free" (default) or "paid": the publication limits (portable/plan.ts)
  // The repository (portable/repo-config.ts reads them).
  EDITOR_GIT_REPO?: string; // "github.com/<owner>/<repo>"
  EDITOR_GIT_BRANCH?: string; // empty = the repository's default branch
  EDITOR_GIT_TOKEN?: string; // a secret, never a [vars] entry
  // `auth` sessions, read by the portable verifier (src/session-verifier.ts).
  // Required: an incomplete or missing set answers 503 everywhere.
  EDITOR_AUTH_ISSUER?: string;
  EDITOR_AUTH_AUDIENCE?: string;
  EDITOR_AUTH_PUBLIC_KEY?: string;
  EDITOR_AUTH_JWKS_URL?: string;
  EDITOR_AUTH_LOGIN_URL?: string;
  // Overrides for a site whose .ssg.yaml the worker cannot read (site-model.ts).
  EDITOR_CONTENT_DIRS?: string; // comma-separated repository folders
  EDITOR_MEDIA_DIR?: string; // the folder pasted images go to
  EDITOR_COMPONENTS_URL?: string; // where the site publishes components.json (same origin; default /components.json)
}

// MODULE_ID: module.json's id — the permission namespace, the log and
// rate-limit prefix, the MCP server's name. PREFIX: EDITOR_AUTH_*, EDITOR_GIT_*.
export const MODULE_ID = "editor-ui";
export const PREFIX = "EDITOR";
export const DEFAULT_BASE = "/editor";

// basePath: EDITOR_BASE normalised, or null when it is not a plain path.
export const basePath = (env: Env): string | null => normalizeBase(env.EDITOR_BASE, DEFAULT_BASE);
