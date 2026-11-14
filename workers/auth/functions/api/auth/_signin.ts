// The ways a person can sign in, and the one step that turns a verified person
// into a session. A leading underscore keeps this file out of the Pages route
// table.
//
// Seams for later tickets, so they add rather than reshape:
//   - signInMethods() (_provider-methods.ts) lists the password and the login
//     providers of GO-151: the login page and the invitation page render one
//     block per method from this list, and the routes refuse a method that is
//     not in it. GO-139 adds Cloudflare Access there.
//   - GO-140 (second factor): startSession() is the ONE function every
//     first-factor success calls — the password (login.ts), each login
//     provider, Cloudflare Access, an accepted invitation — and the second
//     factor's step sits in it (_mfa-signin.ts), so no way in can skip it and
//     none is assumed to have checked a second factor itself. It answers one
//     of three things; signInRedirect() turns them into the browser's next
//     page, for the flows that end in a redirect. openSession() is the
//     session itself, also called after the challenge (mfa/verify.ts).

import { emitEvent } from "./_audit";
import { issueCookies } from "./_context";
import type { Ctx, Issued } from "./_context";
import { redirect } from "./_lib";
import { markSessionFactor } from "./_mfa-policy";
import { CHALLENGE_PAGE, ENROL_PAGE, mustEnrol, secondFactorStep } from "./_mfa-signin";
import type { Challenge } from "./_mfa-signin";
import type { SecondFactor } from "./_mfa-verify";
import { withParams } from "./_next";
import { PASSWORD_METHOD } from "./_provider-policy";
import { createSession, revokeSession } from "./_sessions";
import type { Session } from "./_sessions";
import type { User } from "./_users";

export { methodEnabled, passwordEnabled, signInMethods } from "./_provider-methods";
export type { SignInMethod } from "./_provider-methods";

// Started: a session (`enrol`: a role the account holds needs a second factor
// it has not set up yet — send a browser to enrolment), a session that could
// not be issued (tooLarge), or the second factor's challenge instead.
export type Opened = { ok: true; session: Session; issued: Issued; method: string; enrol: boolean } | { ok: false; tooLarge: string };
export type Started = Opened | Challenge;

// startSession: an already verified, active user passed a first factor by
// `method` ("password", a provider id, …); `next` is where a browser goes
// after a second-factor challenge, when there is one.
export async function startSession(ctx: Ctx, user: User, method = PASSWORD_METHOD, next = ""): Promise<Started> {
  return (await secondFactorStep(ctx, user, method, next)) ?? openSession(ctx, user, method, null);
}

// openSession creates the session and signs its cookies. `method` is stored
// with the session (the per-role method policy reads it at every token
// issue), returned and written to the audit trail; `factor` is the second
// factor passed on the way in, if any (the second-factor policy reads it).
// When the permissions do not fit in a cookie the session is ended at once
// and the reason returned (see _context.ts issueCookies).
export async function openSession(ctx: Ctx, user: User, method: string, factor: SecondFactor | null): Promise<Opened> {
  const { session, refreshToken } = await createSession(ctx.env, user.id, ctx.request, ctx.s.sessionSeconds, ctx.now, method);
  if (factor) await markSessionFactor(ctx.env, session.id, factor, ctx.now);
  const issued = await issueCookies(ctx, user, session, refreshToken);
  if (issued.tooLarge) {
    await revokeSession(ctx.env, session.id, "permissions_too_large", ctx.now);
    return { ok: false, tooLarge: issued.tooLarge };
  }
  await emitEvent(ctx.env, { kind: "signin", userId: user.id, method, detail: { sessionId: session.id, ...(factor ? { secondFactor: factor } : {}) } }, ctx.now);
  return { ok: true, session, issued, method, enrol: await mustEnrol(ctx, user) };
}

// signInRedirect: where a browser goes after startSession, with the cookies
// to set (`cookies` first: a flow cookie to clear, say) — `location` on a
// session, enrolment first when a role needs a factor the account lacks, the
// challenge page with the pending handle, or the login page when the
// permissions did not fit.
export function signInRedirect(ctx: Ctx, started: Started, location: string, cookies: string[] = []): Response {
  if (started.ok === "second_factor") return redirect(CHALLENGE_PAGE, [...cookies, started.cookie]);
  if (!started.ok) return redirect(withParams(ctx.s.loginUrl, { error: "permissions_too_large" }), cookies);
  const to = started.enrol ? withParams(ENROL_PAGE, { required: "1", next: location }) : location;
  return redirect(to, [...cookies, ...started.issued.cookies]);
}
