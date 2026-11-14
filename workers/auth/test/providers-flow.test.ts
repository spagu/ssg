// GO-151, the round trip with each of the five providers against its scripted
// fake: a linked identity signs in; state, nonce, code and redirect URI are
// each refused when wrong, replayed or expired; cancelled consent and provider
// errors land on the login page with their own message.
import { test } from "node:test";
import assert from "node:assert/strict";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { Browser, useClock } from "./fakes.ts";
import { ALL, OIDC } from "./providers-fake.ts";
import type { Pid } from "./providers-fake.ts";
import { answer, callbackGet, count, errorOf, events, fakeProviders, providerEnv, providerUser, roundTrip, startAt } from "./go151.ts";

const PAT = { sub: "4711", email: "pat@example.com", verified: true, name: "Pat" };

async function setup(p: Pid) {
  const clock = useClock();
  jwksCache.clear();
  const env = providerEnv();
  const userId = await providerUser(env, "pat@example.com", p, PAT.sub);
  return { clock, env, userId, fake: fakeProviders(), browser: new Browser() };
}

for (const p of ALL) {
  test(`${p}: a linked identity signs in — session cookies, the method on the session, an audit event, the flow used up`, async () => {
    const { env, userId, fake, browser } = await setup(p);
    try {
      const { started, res, location } = await roundTrip(env, fake, browser, p, PAT, { next: "/auth/account" });
      assert.equal(started.status, 303);
      assert.match(started.headers.getSetCookie()[0], new RegExp(`^__Host-ssg-auth-flow-${p}=[A-Za-z0-9_-]{43}; Max-Age=600; Path=/; Secure; HttpOnly; SameSite=Lax$`));
      assert.equal(res.status, 303, await res.clone().text());
      assert.equal(location, "/auth/account");
      assert.ok(browser.jar.has("__Host-ssg-auth") && browser.jar.has("__Secure-ssg-auth-refresh"));
      assert.ok(!browser.jar.has(`__Host-ssg-auth-flow-${p}`), "flow cookie cleared");
      assert.equal(count(env, "SELECT COUNT(*) AS n FROM oauth_flows"), 0);
      assert.equal(count(env, `SELECT COUNT(*) AS n FROM session_methods WHERE method = '${p}'`), 1);
      const signin = events(env, "signin");
      assert.deepEqual([signin.length, signin[0].user_id, signin[0].method], [1, userId, p]);
      // The token request carried exactly what the protocol asks for.
      const tokenCall = fake.calls.find((c) => /token|access_token/.test(c.url)) as { body: string };
      const form = new URLSearchParams(tokenCall.body);
      assert.equal(form.get("redirect_uri"), `https://example.com/api/auth/oauth/${p}/callback`);
      assert.equal(form.has("code_verifier"), p !== "apple", "PKCE everywhere but Apple");
    } finally {
      fake.restore();
    }
  });

  test(`${p}: wrong, missing, replayed and expired state are refused, and each attempt uses the flow up`, async () => {
    const { clock, env, fake, browser } = await setup(p);
    try {
      const started = await startAt(env, browser, p);
      const back = await fake.authorize(started.headers.get("location") as string, PAT);
      const flowCookie = browser.jar.get(`__Host-ssg-auth-flow-${p}`) as string;
      const tampered = { ...back, path: back.path.replace(/state=[^&]+/, "state=wrong"), form: back.form && { ...back.form, state: "wrong" } };
      const wrong = await answer(env, browser, p, tampered);
      assert.equal(errorOf(wrong.headers.get("location") as string), "oauth_state_invalid");
      // The right state now finds nothing: the wrong one used the flow up.
      browser.jar.set(`__Host-ssg-auth-flow-${p}`, flowCookie);
      assert.equal(errorOf((await answer(env, browser, p, back)).headers.get("location") as string), "oauth_state_invalid");

      // A complete success, then the same answer again with the cookie put back.
      const ok = await roundTrip(env, fake, browser, p, PAT);
      assert.equal(errorOf(ok.location), null);
      const handle = ok.started.headers.getSetCookie()[0].split(";")[0].split("=")[1];
      const replay = new Browser();
      replay.jar.set(`__Host-ssg-auth-flow-${p}`, handle);
      assert.equal(errorOf((await answer(env, replay, p, ok.back)).headers.get("location") as string), "oauth_state_invalid");

      // No cookie at all (another browser), and an expired flow.
      const s2 = await startAt(env, browser, p);
      const b2 = await fake.authorize(s2.headers.get("location") as string, PAT);
      assert.equal(errorOf((await answer(env, new Browser(), p, b2)).headers.get("location") as string), "oauth_state_invalid");
      const s3 = await startAt(env, browser, p);
      const b3 = await fake.authorize(s3.headers.get("location") as string, PAT);
      clock.advance(601_000);
      assert.equal(errorOf((await answer(env, browser, p, b3)).headers.get("location") as string), "oauth_state_invalid");
      assert.equal(events(env, "signin_refused").filter((e) => e.detail.reason === "oauth_state_invalid").length, 5);
    } finally {
      fake.restore();
    }
  });

  test(`${p}: a code used twice, a code minted for another redirect URI, and a callback on another host are refused`, async () => {
    const { env, fake, browser } = await setup(p);
    try {
      const first = await roundTrip(env, fake, browser, p, PAT);
      assert.equal(errorOf(first.location), null);
      const usedCode = /code=([^&]+)/.exec(first.back.path)?.[1] ?? encodeURIComponent((first.back.form as Record<string, string>).code);
      const again = await startAt(env, browser, p);
      const fresh = await fake.authorize(again.headers.get("location") as string, PAT);
      const replayed = { path: fresh.path.replace(/code=[^&]+/, `code=${usedCode}`), form: fresh.form && { ...fresh.form, code: decodeURIComponent(usedCode) } };
      assert.equal(errorOf((await answer(env, browser, p, replayed)).headers.get("location") as string), "provider_failed");

      const s = await startAt(env, browser, p);
      const b = await fake.authorize(s.headers.get("location") as string, PAT);
      for (const c of fake.codes.values()) c.redirectUri = "https://evil.example.com/callback";
      assert.equal(errorOf((await answer(env, browser, p, b)).headers.get("location") as string), "provider_failed");

      const s2 = await startAt(env, browser, p);
      const b2 = await fake.authorize(s2.headers.get("location") as string, PAT);
      const path = b2.form ? `${b2.path}?code=${encodeURIComponent(b2.form.code)}&state=${encodeURIComponent(b2.form.state)}` : b2.path;
      const elsewhere = new Request(`https://other.example.com${path}`, { headers: { cookie: browser.cookieHeader() } });
      const res = await (callbackGet as (c: unknown) => Promise<Response>)({ request: elsewhere, env, params: { provider: p } });
      assert.equal(errorOf(res.headers.get("location") as string), "provider_failed");
      assert.ok(events(env, "signin_refused").some((e) => e.detail.cause === "redirect URI mismatch"));
    } finally {
      fake.restore();
    }
  });

  test(`${p}: cancelled consent, another provider error and a missing code`, async () => {
    const { env, fake, browser } = await setup(p);
    try {
      for (const [error, code] of [[p === "apple" ? "user_cancelled_authorize" : "access_denied", "oauth_cancelled"], ["server_error", "provider_failed"], ["", "provider_failed"]]) {
        const s = await startAt(env, browser, p, { next: "/auth/account" });
        const b = await fake.authorize(s.headers.get("location") as string, PAT);
        const state = b.state;
        const back = b.form ? { path: b.path, form: { state, ...(error ? { error } : {}) } } : { path: `${b.path.split("?")[0]}?state=${state}${error ? `&error=${error}` : ""}` };
        const loc = (await answer(env, browser, p, back)).headers.get("location") as string;
        assert.equal(errorOf(loc), code, `${p} ${error}`);
        assert.equal(new URL(loc, "https://example.com").searchParams.get("next"), "/auth/account", "next survives a refusal");
      }
      assert.equal(fake.calls.length, 0, "nothing was exchanged");
    } finally {
      fake.restore();
    }
  });
}

for (const p of OIDC) {
  test(`${p}: an ID token with another nonce is refused`, async () => {
    const { env, fake, browser } = await setup(p);
    try {
      fake.tweak.claims = (c) => (c.nonce = "someone-elses-nonce");
      assert.equal(errorOf((await roundTrip(env, fake, browser, p, PAT)).location), "provider_failed");
      assert.ok(!browser.jar.has("__Host-ssg-auth"));
    } finally {
      fake.restore();
    }
  });
}
