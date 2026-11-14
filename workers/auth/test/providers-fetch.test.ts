// GO-151, talking to providers: https only, hard-coded hosts, no redirect
// followed (to another host or anywhere), a size cap and a time limit — each
// a refused sign-in; and the provider APIs' own failure answers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { okJson, providerFetch, providerLimits } from "../functions/api/auth/_provider-fetch.ts";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { clientSecret } from "../functions/api/auth/providers/_apple.ts";
import { Browser, useClock } from "./fakes.ts";
import { ALL } from "./providers-fake.ts";
import type { Pid, Tweak } from "./providers-fake.ts";
import { errorOf, events, fakeProviders, providerEnv, providerUser, roundTrip } from "./go151.ts";

const HOSTS = ["api.example.com"];
const using = async (fetchImpl: typeof fetch, run: () => Promise<void>) => {
  const real = globalThis.fetch;
  globalThis.fetch = fetchImpl;
  try {
    await run();
  } finally {
    globalThis.fetch = real;
  }
};

test("providerFetch: refuses http and other hosts without sending anything; never follows a redirect", async () => {
  let sent = 0;
  await using((async () => (sent++, new Response(null, { status: 302, headers: { location: "https://evil.example.com/" } }))) as typeof fetch, async () => {
    assert.deepEqual(await providerFetch("http://api.example.com/x", HOSTS), { ok: false, detail: "refused api.example.com/x: not https" });
    assert.deepEqual(await providerFetch("https://evil.example.com/x?code=secret", HOSTS), { ok: false, detail: "refused evil.example.com/x: host not allowed" });
    assert.equal(sent, 0);
    assert.deepEqual(await providerFetch("https://api.example.com/x", HOSTS), { ok: false, detail: "api.example.com/x: redirected" });
  });
  await using((async (_u: string, init: RequestInit) => {
    assert.equal(init.redirect, "error");
    throw new TypeError("redirect");
  }) as typeof fetch, async () => {
    assert.deepEqual(await providerFetch("https://api.example.com/y", HOSTS), { ok: false, detail: "api.example.com/y: request failed or redirected" });
  });
});

test("providerFetch: size cap (declared and streamed), time limit, JSON only, a body that breaks off", async () => {
  const big = () => new Response(new Blob(["[", "1,".repeat(providerLimits.maxBytes), "1]"]).stream());
  await using((async () => big()) as typeof fetch, async () => {
    assert.deepEqual(await providerFetch("https://api.example.com/a", HOSTS), { ok: false, detail: `api.example.com/a: answer larger than ${providerLimits.maxBytes} bytes` });
  });
  await using((async () => new Response("{}", { headers: { "content-length": "99999999" } })) as typeof fetch, async () => {
    assert.equal((await providerFetch("https://api.example.com/b", HOSTS)).ok, false);
  });
  await using((async () => new Response("<html>")) as typeof fetch, async () => {
    assert.deepEqual(await providerFetch("https://api.example.com/c", HOSTS), { ok: false, detail: "api.example.com/c: answer is not JSON (status 200)" });
  });
  await using((async () => new Response(null, { status: 204 })) as typeof fetch, async () => {
    assert.equal((await providerFetch("https://api.example.com/e", HOSTS)).ok, false, "an empty body is not JSON");
  });
  const broken = new ReadableStream({ pull: (c) => c.error(new Error("connection reset")) });
  await using((async () => new Response(broken)) as typeof fetch, async () => {
    assert.equal((await providerFetch("https://api.example.com/d", HOSTS)).ok, false);
  });
  const saved = providerLimits.timeoutMs;
  providerLimits.timeoutMs = 20;
  try {
    await using(((_u: string, init: RequestInit) => new Promise((_, reject) => init.signal?.addEventListener("abort", () => reject(init.signal?.reason)))) as typeof fetch, async () => {
      assert.deepEqual(await providerFetch("https://api.example.com/slow", HOSTS), { ok: false, detail: "api.example.com/slow: no answer within 20 ms" });
    });
  } finally {
    providerLimits.timeoutMs = saved;
  }
  assert.deepEqual(okJson({ ok: true, status: 400, data: { error: "invalid_grant" } }, "token"), { ok: false, detail: "token: status 400, error invalid_grant" });
  assert.deepEqual(okJson({ ok: true, status: 200, data: [1] }, "x"), { ok: false, detail: "x: status 200" });
  assert.deepEqual(okJson({ ok: true, status: 500, data: null }, "x"), { ok: false, detail: "x: status 500" });
});

async function refused(p: Pid, tweak: Tweak) {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  await providerUser(env, "pat@example.com", p, "4711");
  const fake = fakeProviders();
  fake.tweak = tweak;
  try {
    const browser = new Browser();
    const { location } = await roundTrip(env, fake, browser, p, { sub: "4711", email: "pat@example.com", verified: true });
    return { error: errorOf(location), signedIn: browser.jar.has("__Host-ssg-auth"), cause: events(env, "signin_refused")[0]?.detail.cause as string };
  } finally {
    fake.restore();
  }
}

for (const p of ALL) {
  test(`${p}: an endpoint that redirects elsewhere, answers too much, too slowly, or refuses the code → refused`, async () => {
    const saved = providerLimits.timeoutMs;
    providerLimits.timeoutMs = 20;
    try {
      for (const tweak of [{ redirect: true }, { oversize: "declared" }, { oversize: "streamed" }, { slow: true }, { tokenAnswer: { status: 400, body: { error: "invalid_grant" } } }, { tokenAnswer: { status: 200, body: { token_type: "bearer" } } }] as Tweak[]) {
        const r = await refused(p, tweak);
        assert.deepEqual([r.error, r.signedIn], ["provider_failed", false], `${p} ${JSON.stringify(tweak)}`);
        assert.ok(r.cause && !/secret|at-|code-/.test(r.cause), r.cause);
      }
    } finally {
      providerLimits.timeoutMs = saved;
    }
  });
}

test("github: no numeric id, no e-mail list, no primary address; facebook: no id", async () => {
  assert.equal((await refused("github", { emails: { message: "nope" } })).error, "provider_failed");
  const noPrimary = await refused("github", { emails: [{ email: "x@example.com", primary: false, verified: true }] });
  assert.equal(noPrimary.error, null, "a linked account signs in without an address");
  const env = providerEnv();
  useClock();
  const fake = fakeProviders();
  try {
    await providerUser(env, "pat@example.com", "github", "4711");
    fake.tweak = { tokenAnswer: { status: 200, body: { access_token: "unknown-token" } } };
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "github", { sub: "4711" })).location), "provider_failed");
    fake.tweak = { tokenAnswer: { status: 200, body: { access_token: "unknown-token" } } };
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "facebook", { sub: "4711" })).location), "provider_failed");
  } finally {
    fake.restore();
  }
});

test("apple: the client secret is an ES256 JWT for the team, key and Services ID; a key that is not P-256 fails the sign-in", async () => {
  assert.equal(await clientSecret({ pem: "AAAA", keyId: "K", teamId: "T" }, "c", 0), null);
  const r = await refused("apple", {});
  assert.equal(r.error, null, "the fake's token endpoint verified the signed client secret");
  const env = providerEnv({ AUTH_APPLE_PRIVATE_KEY: "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----" } as never);
  useClock();
  const fake = fakeProviders();
  try {
    await providerUser(env, "pat@example.com", "apple", "4711");
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "apple", { sub: "4711" })).location), "provider_failed");
    assert.match(events(env, "signin_refused")[0].detail.cause as string, /not a P-256 private key/);
  } finally {
    fake.restore();
  }
});
