// The hard-bounce circuit breaker. A mail server that answers 550 "user
// unknown" to everyone — a broken relay, a misrouted MX, a provider having a
// bad day — looks to the bounce classifier exactly like a list full of dead
// addresses, and suppression is (for `hard`) absolute. One such batch must not
// wipe a list, so within one batch:
//
//   tripped = hard ≥ NEWSLETTER_BOUNCE_BREAKER_MIN (default 5)
//             AND hard ≥ NEWSLETTER_BOUNCE_BREAKER_PERCENT (default 50) % of the
//                 messages handed to the transport in this batch so far
//
// checked after every hard bounce. When it trips, the batch stops, the
// campaign is paused with the reason (as for a render error), the batch's
// permanent-failure hooks are never run — so nothing from it is suppressed —
// and its hard-bounced rows go back to `retry` without counting the attempt,
// so a resume after the server is fixed tries them again.
//
// Hooks of a batch are therefore deferred to its end (engine-deliver.ts). The
// price: a consumer killed by the platform mid-batch loses that batch's
// suppressions; the rows stay `failed` with the server's words and the next
// campaign bounces and suppresses them.

import { classifyPermanent } from "./bounce-classify";
import type { PermanentFailure } from "./engine-hooks";
import type { Env } from "./lib";
import { intVar } from "./lib";

export const DEFAULT_BREAKER_MIN = 5;
export const DEFAULT_BREAKER_PERCENT = 50;

export interface Breaker {
  min: number;
  percent: number;
  attempted: number; // messages handed to the transport in this batch
  hard: string[]; // subscriber ids refused as unknown mailboxes
  failures: PermanentFailure[]; // every permanent failure, for the deferred hooks
}

export function breakerFor(env: Env): Breaker {
  return {
    min: intVar(env.NEWSLETTER_BOUNCE_BREAKER_MIN, DEFAULT_BREAKER_MIN),
    percent: Math.min(100, intVar(env.NEWSLETTER_BOUNCE_BREAKER_PERCENT, DEFAULT_BREAKER_PERCENT)),
    attempted: 0,
    hard: [],
    failures: [],
  };
}

// notePermanent records one permanent failure and reports whether the
// breaker trips with it.
export function notePermanent(b: Breaker, f: PermanentFailure): boolean {
  b.failures.push(f);
  if (classifyPermanent(f.detail) !== "hard") return false;
  b.hard.push(f.subscriberId);
  return b.hard.length >= b.min && b.hard.length * 100 >= b.percent * b.attempted;
}

export const breakerReason = (b: Breaker): string =>
  `bounces: ${b.hard.length} of ${b.attempted} messages in one batch were refused as unknown mailboxes; ` +
  "nothing was suppressed — check the mail server, then resume to try them again";
