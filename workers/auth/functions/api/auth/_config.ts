// Every tunable of the auth worker in one place, read from the environment
// with its default. A leading underscore keeps this file out of the route table.

import { intVar } from "./_lib";
import type { Env } from "./_lib";

// DEFAULT_ITERATIONS is the PBKDF2-SHA256 cost of a new password hash: the
// highest count Cloudflare's production runtime accepts (100,000 — see the
// README's measurement). Lower than current guidance for PBKDF2-SHA256; the
// lockouts, the minimum length and (later) a second factor make up for it.
export const DEFAULT_ITERATIONS = 100_000;
// MIN_ITERATIONS: a lower AUTH_PBKDF2_ITERATIONS is a typo, not a choice.
export const MIN_ITERATIONS = 10_000;

export interface Settings {
  issuer: string;
  audience: string;
  tokenSeconds: number;
  sessionSeconds: number;
  graceSeconds: number;
  iterations: number;
  passwordMin: number;
  passwordMax: number;
  lockAccount: number;
  lockIP: number;
  lockBaseSeconds: number;
  lockMaxSeconds: number;
  turnstileAfter: number;
  turnstileSecret: string | undefined;
  loginUrl: string;
  afterLoginUrl: string;
}

// settings resolves the configuration for one request. The issuer and the
// audience default to this site, so a token minted here is refused by a
// worker configured for another host.
export function settings(env: Env, request: Request): Settings {
  const url = new URL(request.url);
  return {
    issuer: env.AUTH_ISSUER || `${url.origin}/api/auth`,
    audience: env.AUTH_AUDIENCE || url.host,
    tokenSeconds: intVar(env.AUTH_TOKEN_MINUTES, 10) * 60,
    sessionSeconds: intVar(env.AUTH_SESSION_DAYS, 14) * 86_400,
    graceSeconds: intVar(env.AUTH_REFRESH_GRACE_SECONDS, 30),
    iterations: Math.max(MIN_ITERATIONS, intVar(env.AUTH_PBKDF2_ITERATIONS, DEFAULT_ITERATIONS)),
    passwordMin: intVar(env.AUTH_PASSWORD_MIN, 12),
    passwordMax: intVar(env.AUTH_PASSWORD_MAX, 256),
    lockAccount: intVar(env.AUTH_LOCK_ACCOUNT_FAILURES, 5),
    lockIP: intVar(env.AUTH_LOCK_IP_FAILURES, 20),
    lockBaseSeconds: intVar(env.AUTH_LOCK_BASE_SECONDS, 30),
    lockMaxSeconds: intVar(env.AUTH_LOCK_MAX_SECONDS, 3600),
    turnstileAfter: intVar(env.AUTH_TURNSTILE_AFTER, 3),
    // Every worker in one Pages project shares one environment (#325), so the
    // prefixed name is this worker's own and TURNSTILE_SECRET only a fallback.
    turnstileSecret: env.AUTH_TURNSTILE_SECRET || env.TURNSTILE_SECRET,
    loginUrl: env.AUTH_LOGIN_URL || "/auth/login",
    afterLoginUrl: env.AUTH_AFTER_LOGIN_URL || "/",
  };
}
