// The Cloudflare plan a panel worker publishes within (GO-186):
// {PREFIX}_PLAN = "free" (the default, also when unset or empty) or "paid".
// It selects the repository layer's PLAN_LIMITS (files and bytes per
// publication, measured in GO-168), how many requests one invocation may
// make to the repository host, and how large a request body may be. Any
// other value is a configuration error: app.ts answers 503 bad_plan on every
// path, naming the variable, so a typo never falls back to Free (or Paid) without anyone noticing.
// Shared by the panel workers: the prefix comes from the template's src/http.ts.

import { MAX_BODY } from "./http-kit";
import { MB, PLAN_LIMITS } from "./repo-provider";
import type { ChangeLimits } from "./repo-provider";
import { PREFIX } from "../src/http";
import type { Env } from "../src/http";
import { OpError } from "./ops";

export type Plan = keyof typeof PLAN_LIMITS;

// SUBREQUEST_BUDGET: requests to the repository host per invocation.
// Cloudflare's Workers limits (developers.cloudflare.com/workers/platform/limits,
// read 2026-11-15, page last updated 2026-11-13): Free 50 subrequests per
// invocation, Paid 10,000 by default. One publication costs files + 5 +
// directories, plus 6 per retry after a lost race, at most 3 attempts (GO-168).
//   free: 45 = 50 - 5 kept for the `auth` key fetch and the panel's assets; the
//         worst publication the Free limits allow (10 files in 10 directories,
//         two lost races) takes 37.
//   paid: 125 = the worst publication the Paid limits allow, 50 + 5 + 50 + 2 * 6
//         = 117, plus the same 8 for the reads around it that Free keeps
//         (45 - 37). Far below 10,000 on purpose: every request also spends
//         the token's hourly quota at the repository host (5,000 on GitHub).
export const SUBREQUEST_BUDGET: Record<Plan, number> = { free: 45, paid: 125 };

// BODY_LIMIT: one request body (a publication, an MCP call).
//   free: MAX_BODY, 8 MiB (http-kit.ts): a change at the Free limits as base64
//         (2 MiB -> 2.7 MiB) with room for the JSON around it.
//   paid: 28 MiB = the Paid change limit as base64 (20 MiB -> 26.7 MiB) plus
//         1.3 MiB for texts, paths and the message. GO-168 measured about 2.4
//         times the body in memory, so about 67 MB of the isolate's 128 MB.
export const BODY_LIMIT: Record<Plan, number> = { free: MAX_BODY, paid: 28 * MB };

// ActivePlan: what the worker applies, reported by api/health, api/site and
// api/media so the panel shows the numbers the worker enforces.
export interface ActivePlan { plan: Plan; limits: ChangeLimits; subrequests: number; maxBody: number }

export const PLAN_VAR = `${PREFIX}_PLAN`;

// planOf: the active plan, or the problem naming the variable.
export function planOf(env: Env): { ok: true; active: ActivePlan } | { ok: false; problem: string } {
  const raw = (env as Record<string, unknown>)[PLAN_VAR];
  const name = typeof raw === "string" ? raw.trim().toLowerCase() || "free" : raw === undefined ? "free" : "";
  if (name !== "free" && name !== "paid") return { ok: false, problem: `${PLAN_VAR} must be "free" or "paid" (empty = free)` };
  return { ok: true, active: { plan: name, limits: PLAN_LIMITS[name], subrequests: SUBREQUEST_BUDGET[name], maxBody: BODY_LIMIT[name] } };
}

// activePlan: for an operation; 503 bad_plan otherwise (app.ts refuses such
// a request before any operation runs; this keeps an operation called
// directly just as strict).
export function activePlan(env: Env): ActivePlan {
  const p = planOf(env);
  if (!p.ok) throw new OpError("bad_plan", 503, p.problem);
  return p.active;
}
