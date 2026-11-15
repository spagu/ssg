// GET {base}/api/health — whether the worker is wired up, from its own
// configuration only: no request to the repository host (the summary does
// that), so it answers the same whoever asks and costs nothing. It names
// what is missing, never a value — the token above all — and the plan whose
// limits apply ({PREFIX}_PLAN, portable/plan.ts).

import { activePlan } from "../portable/plan";
import type { ActivePlan } from "../portable/plan";
import { repoConfigFrom } from "../portable/repo-config";
import { PREFIX } from "./http";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";

export interface Health {
  ok: boolean;
  auth: "auth";
  repository: { configured: true; provider: string; branch: string | null } | { configured: false; problem: string };
  overrides: { contentDirs: boolean; mediaDir: boolean };
  plan: ActivePlan;
  bindings: { assets: boolean; rateLimiter: boolean };
}

export const healthOp: Op<void, Health> = {
  name: "health",
  permission: PERM.read,
  run: ({ env }) => {
    const r = repoConfigFrom(env as Record<string, unknown>, PREFIX);
    return {
      ok: r.ok,
      auth: "auth", // the gate let the request in, so the set is complete
      repository: r.ok ? { configured: true, provider: r.config.provider, branch: r.config.branch } : { configured: false, problem: r.problem },
      overrides: { contentDirs: !!env.EDITOR_CONTENT_DIRS?.trim(), mediaDir: !!env.EDITOR_MEDIA_DIR?.trim() },
      plan: activePlan(env),
      bindings: { assets: !!env.ASSETS, rateLimiter: !!env.RATE_LIMITER },
    };
  },
};
