// transportReady is the one check every route that sends mail makes before
// doing anything else: a transport is configured, and its settings can work.
// An SMTP setting that can never send (port 25, a port that is not a number,
// a user without a password, a bad EHLO name — see transportProblem in the
// portable mail module) is a clear 503 up front, not a failed send later, and
// costs no test-send budget.

import { json } from "./lib";
import type { Env } from "./lib";
import { transportOf, transportProblem } from "./mail/_transport";

export const NO_TRANSPORT_DETAIL = "configure Resend, MailChannels or SMTP (see README)";

// transportReady returns null when mail can be handed to the transport, else
// the JSON answer: 503 no_transport or 503 transport_unusable {detail}.
export function transportReady(env: Env): Response | null {
  if (!transportOf(env)) return json({ error: "no_transport", detail: NO_TRANSPORT_DETAIL }, 503);
  const problem = transportProblem(env);
  return problem ? json({ error: "transport_unusable", detail: problem }, 503) : null;
}
