// Cloudflare Pages Function: the redirect URI of a login provider (GO-151).
//   GET  /api/auth/oauth/:provider/callback?code=…&state=…  (or ?error=…)
//   POST /api/auth/oauth/:provider/callback  — only for a provider that
//        answers with response_mode=form_post (Apple)
//
// This exact URL, on this host, is what each provider's console must list —
// one per provider, compared exactly. GET does the work (_provider-callback.ts)
// and always answers 303: to `next`, /auth/ or /auth/account on success, to
// the login page (the account page for a link) with ?error=<code> otherwise,
// clearing the flow cookie either way.
//
// Why the POST only bounces. Apple POSTs from its own origin, and a browser
// sends no SameSite=Lax cookie with a cross-site POST — so the flow cookie,
// which binds the answer to this browser, is not there. The POST therefore
// checks nothing and changes nothing: it answers 303 to the GET above with
// the same fields in the query, a top-level GET navigation that carries the
// cookie, where every check runs. (No CSRF defence on it for the same reason:
// it is cross-site by design and can do no more than a crafted GET link.)
// A provider that does not use form_post answers the POST 404.
//
// A provider this site does not offer answers 404 not_found; one with wrong
// variables 503 provider_misconfigured.

import { param } from "../../_admin";
import { prepare } from "../../_context";
import { json, redirect } from "../../_lib";
import type { Env } from "../../_lib";
import { callbackPath, providerOr } from "../../_provider";
import { callback } from "../../_provider-callback";
import { readBody, str } from "../../_request";

const FIELDS = ["code", "state", "error", "user"] as const;

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const found = providerOr(env, param(params, "provider"));
  if (found instanceof Response) return found;
  const q = new URL(request.url).searchParams;
  const [code, state, error, user] = FIELDS.map((k) => (q.get(k) ?? "").slice(0, 4096));
  return callback(ctx, found.provider, found.setup, { code, state, error, user });
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const found = providerOr(env, param(params, "provider"));
  if (found instanceof Response) return found;
  if (!found.provider.formPost) return json({ error: "not_found" }, 404);
  const b = (await readBody(request)) ?? {};
  const q = new URLSearchParams();
  for (const k of FIELDS) {
    const v = str(b[k]).slice(0, 4096);
    if (v) q.set(k, v);
  }
  return redirect(`${callbackPath(found.provider.id)}?${q}`);
};
