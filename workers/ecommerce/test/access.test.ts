// Cloudflare Access, the other way in.
//
// The verifier fetches the team's public keys and checks a signature made by
// Cloudflare. The tests stand in for Cloudflare with a key pair of their own,
// which is the only way to test a signature check without a real IdP.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { resetAccessCache, verifyAccessJwt } from "../functions/api/shop/_access";
import { requireAdmin } from "../functions/api/shop/_auth";
import { base64url } from "../functions/api/shop/_lib";
import { freshShop, ORIGIN } from "./_helpers";

const TEAM = "myteam";
const AUD = "aud-tag";
const realFetch = globalThis.fetch;

let keyPair: CryptoKeyPair;
let jwks: string;

async function makeKeys(): Promise<void> {
  keyPair = (await crypto.subtle.generateKey(
    { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const exported = (await crypto.subtle.exportKey("jwk", keyPair.publicKey)) as JsonWebKey;
  jwks = JSON.stringify({ keys: [{ ...exported, kid: "kid-1", alg: "RS256", use: "sig" }] });
}

async function token(payload: Record<string, unknown>, kid = "kid-1"): Promise<string> {
  const encode = (value: unknown): string => base64url(new TextEncoder().encode(JSON.stringify(value)));
  const head = encode({ alg: "RS256", typ: "JWT", kid });
  const body = encode(payload);
  const signature = await crypto.subtle.sign(
    "RSASSA-PKCS1-v1_5",
    keyPair.privateKey,
    new TextEncoder().encode(`${head}.${body}`),
  );
  return `${head}.${body}.${base64url(new Uint8Array(signature))}`;
}

function valid(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const now = Math.floor(Date.now() / 1000);
  return {
    aud: [AUD],
    email: "owner@example.com",
    sub: "access-sub-1",
    iss: `https://${TEAM}.cloudflareaccess.com`,
    exp: now + 600,
    iat: now - 10,
    ...overrides,
  };
}

function request(jwt: string | null, header = "cf-access-jwt-assertion"): Request {
  return new Request(`${ORIGIN}/api/shop/admin/me`, {
    headers: jwt ? { [header]: jwt } : {},
  });
}

beforeEach(async () => {
  await freshShop();
  resetAccessCache();
  await makeKeys();
  globalThis.fetch = (async (url: unknown) => {
    if (String(url).includes("/cdn-cgi/access/certs")) return new Response(jwks);
    throw new Error(`unexpected fetch to ${url}`);
  }) as unknown as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("verifying an Access token", () => {
  it("accepts one signed by the team's key", async () => {
    const verdict = await verifyAccessJwt(request(await token(valid())), { team: TEAM, aud: AUD });
    expect(verdict).toMatchObject({ ok: true, email: "owner@example.com", sub: "access-sub-1" });
  });

  it("accepts the team spelled out in full", async () => {
    const verdict = await verifyAccessJwt(request(await token(valid())), {
      team: `${TEAM}.cloudflareaccess.com`,
      aud: AUD,
    });
    expect(verdict.ok).toBe(true);
  });

  it("reads the cookie when the header is absent", async () => {
    const jwt = await token(valid());
    const withCookie = new Request(`${ORIGIN}/api/shop/admin/me`, {
      headers: { cookie: `CF_Authorization=${jwt}` },
    });
    expect((await verifyAccessJwt(withCookie, { team: TEAM, aud: AUD })).ok).toBe(true);
  });

  it("refuses a token for another application", async () => {
    const verdict = await verifyAccessJwt(request(await token(valid({ aud: ["someone-else"] }))), {
      team: TEAM,
      aud: AUD,
    });
    expect(verdict.ok).toBe(false);
  });

  it("refuses a token from another team", async () => {
    const verdict = await verifyAccessJwt(
      request(await token(valid({ iss: "https://elsewhere.cloudflareaccess.com" }))),
      { team: TEAM, aud: AUD },
    );
    expect(verdict.ok).toBe(false);
  });

  it("refuses an expired one", async () => {
    const past = Math.floor(Date.now() / 1000) - 60;
    const verdict = await verifyAccessJwt(request(await token(valid({ exp: past }))), { team: TEAM, aud: AUD });
    expect(verdict.ok).toBe(false);
  });

  it("refuses one signed by a key the team does not publish", async () => {
    const jwt = await token(valid());
    await makeKeys(); // the team rotates to a different key entirely
    resetAccessCache();
    expect((await verifyAccessJwt(request(jwt), { team: TEAM, aud: AUD })).ok).toBe(false);
  });

  it("refuses a token naming a key id nobody published", async () => {
    const jwt = await token(valid(), "kid-unknown");
    expect((await verifyAccessJwt(request(jwt), { team: TEAM, aud: AUD })).ok).toBe(false);
  });

  it("refuses rubbish, and an absent token, without throwing", async () => {
    expect((await verifyAccessJwt(request(null), { team: TEAM, aud: AUD })).ok).toBe(false);
    expect((await verifyAccessJwt(request("a.b"), { team: TEAM, aud: AUD })).ok).toBe(false);
    expect((await verifyAccessJwt(request("not.a.token"), { team: TEAM, aud: AUD })).ok).toBe(false);
  });

  it("says so rather than letting anyone in when the keys cannot be fetched", async () => {
    globalThis.fetch = (async () => new Response("no", { status: 500 })) as typeof fetch;
    resetAccessCache();
    const verdict = await verifyAccessJwt(request(await token(valid())), { team: TEAM, aud: AUD });
    expect(verdict.ok).toBe(false);
  });
});

describe("the shop's gate under Access", () => {
  it("turns a verified token into an identity, owner or staff by configuration", async () => {
    const accessEnv = {
      ...env,
      SHOP_ACCESS_TEAM: TEAM,
      SHOP_ACCESS_AUD: AUD,
      SHOP_ACCESS_OWNERS: "owner@example.com",
    };

    const asOwner = await requireAdmin(request(await token(valid())), accessEnv);
    expect(asOwner).toMatchObject({ role: "owner", via: "access", email: "owner@example.com" });

    // Everyone else who gets through Access is staff: they can look, and the
    // owner-only endpoints still refuse them.
    const asStaff = await requireAdmin(
      request(await token(valid({ email: "helper@example.com", sub: "s2" }))),
      accessEnv,
    );
    expect(asStaff).toMatchObject({ role: "staff", via: "access" });
  });

  it("refuses when Access is configured and no token arrives", async () => {
    const verdict = await requireAdmin(request(null), {
      ...env,
      SHOP_ACCESS_TEAM: TEAM,
      SHOP_ACCESS_AUD: AUD,
    });
    expect(verdict).toBeInstanceOf(Response);
    expect((verdict as Response).status).toBe(401);
  });

  it("refuses everything when neither mode is configured", async () => {
    // A shop with no admin authentication has no admin API, rather than an
    // open one.
    const verdict = await requireAdmin(request(null), {
      ...env,
      SHOP_JWT_SECRET: undefined,
      SHOP_ACCESS_TEAM: undefined,
      SHOP_ACCESS_AUD: undefined,
    });
    expect((verdict as Response).status).toBe(503);
  });
});
