// Cloudflare Pages Function: accept an invitation.
//   POST /api/auth/invite   JSON {token, method?: "password", password, name?}
//                           or the same fields from the plain HTML form of /auth/invite
//
// The invitee chooses how to sign in among signInMethods() (_signin.ts) —
// today only "password" (the default). The password must pass the usual
// rules; a refused one does not use up the invitation. Then, in one
// transaction, the invitation is marked accepted and the account created with
// the roles and groups recorded at invite time that still exist; the person
// is signed in.
// JSON: 201 {user, expiresAt, sessionExpiresAt, skipped: {roles, groups}} +
// session cookies; 410 invitation_invalid (unknown, used, withdrawn or
// expired — one answer for all); 400 unsupported_method / <password rule>;
// 409 account_exists; 503 hashing_unavailable. A form gets a 303: to
// /auth/?welcome=1 (&skipped=<n>) on success, back to /auth/invite?token=…
// &error=<code> otherwise (without the token when it is the token that failed).
// GO-140: when a role of the new account needs a second factor, a form goes
// to /auth/mfa/enrol first (then on to the welcome) and JSON adds `enrol: true`.

import { hashingUnavailable, permissionsTooLarge, preparePost } from "./_context";
import { acceptInvitation } from "./_invites-accept";
import { openByToken } from "./_invites";
import { isToken, json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { withParams } from "./_next";
import { hashPassword, passwordProblem } from "./_password";
import { isForm, readBody, str } from "./_request";
import { methodEnabled, signInRedirect, startSession } from "./_signin";
import type { Opened } from "./_signin";
import { findUserById, publicUser, touchLogin } from "./_users";
import type { User } from "./_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const { s, now } = ctx;
  const form = isForm(request);
  const b = await readBody(request);
  const token = str(b?.token);
  const fail = (error: string, status: number, keepToken = true): Response =>
    form ? redirect(withParams("/auth/invite", { token: keepToken ? token : "", error })) : json({ error }, status);

  const inv = isToken(token) ? await openByToken(env, token, now) : null;
  if (!inv) return fail("invitation_invalid", 410, false);
  if (!methodEnabled(env, b?.method ?? "password")) return fail("unsupported_method", 400);
  const problem = passwordProblem(b?.password, inv.email, s);
  if (problem) return fail(problem, 400);
  let hash: string;
  try {
    hash = await hashPassword(str(b?.password), s.iterations);
  } catch {
    return hashingUnavailable();
  }
  const name = [...(str(b?.name).trim() || inv.name)].slice(0, 200).join("");
  const r = await acceptInvitation(env, inv, token, { name, passwordHash: hash }, now);
  if (r.kind === "invalid") return fail("invitation_invalid", 410, false);
  if (r.kind === "account_exists") return fail("account_exists", 409);

  const user = (await findUserById(env, r.userId)) as User;
  // Accepting is the account's first sign-in: the admin panel's "Last
  // sign-in" must not say "Never" for it (login.ts does the same).
  await touchLogin(env, user.id, now);
  // A new account has no second factor yet, so this is a session (or a
  // refusal) — never the challenge.
  const started = (await startSession(ctx, user)) as Opened;
  const skipped = r.skipped.roles.length + r.skipped.groups.length;
  if (form) return signInRedirect(ctx, started, withParams("/auth/", { welcome: "1", skipped: skipped ? String(skipped) : "" }));
  if (!started.ok) return permissionsTooLarge(started.tooLarge);
  const { session, issued } = started;
  const enrol = started.enrol ? { enrol: true } : {};
  return json({ user: publicUser(user), expiresAt: issued.exp, sessionExpiresAt: session.expires_at, skipped: r.skipped, ...enrol }, 201, {}, issued.cookies);
};
