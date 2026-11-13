// Hooks the send engine calls at points other features act on. The engine
// calls them through `engineHooks` (so tests can observe them) and never lets
// a failing hook stop a batch: it logs and goes on.
//
// GO-127 fills both with bounce handling (src/bounce-*.ts).

import { classifyPermanent } from "./bounce-classify";
import { applyBounces, resetSmtpSoft } from "./bounce-store";
import type { Env } from "./lib";

export interface PermanentFailure {
  email: string;
  subscriberId: string;
  campaignId: string;
  detail: string; // the transport's words, e.g. "smtp 550 at RCPT TO: 5.1.1 user unknown"
}

// onPermanentFailure runs after a recipient's message was refused for good
// and the send was recorded as `failed`. A refusal of the mailbox (SMTP 550 /
// 551 / 553, 5.1.x at RCPT TO) is a hard bounce, a full mailbox (5.2.2) a soft
// one; anything else — policy, content, authentication, an HTTP provider's
// API error — leaves the address alone (bounce-classify.ts says why).
export async function onPermanentFailure(env: Env, failure: PermanentFailure): Promise<void> {
  const kind = classifyPermanent(failure.detail);
  if (!kind) return;
  await applyBounces(env, "smtp", [{ id: null, kind, emails: [failure.email], source: "smtp", detail: failure.detail }]);
}

// onDelivered runs once per batch with the addresses the transport accepted:
// it resets their SMTP soft-bounce counters in one statement.
export async function onDelivered(env: Env, emails: string[]): Promise<void> {
  await resetSmtpSoft(env, emails);
}

export const engineHooks = { onPermanentFailure, onDelivered };
