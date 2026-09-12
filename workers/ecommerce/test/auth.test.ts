// Admin authentication: passwords, tokens, and the session rotation that makes
// a stolen refresh token useless the second time it is used.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import {
  clearLoginFailures,
  consumeRefresh,
  createRefresh,
  endAllSessions,
  hashPassword,
  loginAllowed,
  maybeBootstrap,
  noteLoginFailure,
  readCookie,
  refreshCookie,
  REFRESH_COOKIE,
  requireAdmin,
  requireOwner,
  revokeJti,
  signJwt,
  verifyJwt,
  verifyPassword,
} from "../functions/api/shop/_auth";
import type { AdminIdentity, AdminUserRow } from "../functions/api/shop/_types";
import { freshShop, ORIGIN, seedOwner } from "./_helpers";

beforeEach(async () => {
  await freshShop();
});

async function ownerRow(): Promise<AdminUserRow> {
  await seedOwner();
  return (await env.SHOP_DB.prepare(`SELECT * FROM admin_users LIMIT 1`).first<AdminUserRow>())!;
}

describe("passwords", () => {
  it("verifies the right one and rejects the wrong one", async () => {
    const stored = await hashPassword("correct horse", 1000);
    expect(await verifyPassword("correct horse", stored)).toBe(true);
    expect(await verifyPassword("Correct horse", stored)).toBe(false);
  });

  it("salts, so the same password hashes differently every time", async () => {
    expect(await hashPassword("same", 1000)).not.toBe(await hashPassword("same", 1000));
  });

  it("carries its own parameters, so the cost can be raised later", async () => {
    const stored = await hashPassword("x", 1000);
    expect(stored).toMatch(/^pbkdf2\$sha256\$1000\$/);
    // An old hash with an old iteration count still verifies after the default
    // is raised — which is the point of storing the count.
    expect(await verifyPassword("x", stored)).toBe(true);
  });

  it("refuses a stored value that is not a hash it wrote", async () => {
    expect(await verifyPassword("x", "")).toBe(false);
    expect(await verifyPassword("x", "plaintext")).toBe(false);
    expect(await verifyPassword("x", "pbkdf2$sha256$notanumber$aa$bb")).toBe(false);
  });
});

describe("access tokens", () => {
  it("round-trips a signed token", async () => {
    const user = await ownerRow();
    const token = await signJwt(env, user, ORIGIN);
    const payload = await verifyJwt(env, token, ORIGIN);
    expect(payload?.email).toBe(user.email_lc);
    expect(payload?.role).toBe("owner");
  });

  it("refuses a token signed with another key", async () => {
    const user = await ownerRow();
    const token = await signJwt(env, user, ORIGIN);
    const other = { ...env, SHOP_JWT_SECRET: "a-completely-different-secret-of-length" };
    expect(await verifyJwt(other, token, ORIGIN)).toBe(null);
  });

  it("refuses a token issued for another site", async () => {
    const user = await ownerRow();
    const token = await signJwt(env, user, "https://elsewhere.example");
    expect(await verifyJwt(env, token, ORIGIN)).toBe(null);
  });

  it("refuses rubbish without throwing", async () => {
    expect(await verifyJwt(env, "not.a.token", ORIGIN)).toBe(null);
    expect(await verifyJwt(env, "", ORIGIN)).toBe(null);
    expect(await verifyJwt(env, "a.b", ORIGIN)).toBe(null);
  });

  it("stops accepting a token whose id has been revoked", async () => {
    const user = await ownerRow();
    const token = await signJwt(env, user, ORIGIN);
    const payload = (await verifyJwt(env, token, ORIGIN))!;
    await revokeJti(env, payload.jti, payload.exp);
    expect(await verifyJwt(env, token, ORIGIN)).toBe(null);
  });
});

describe("refresh tokens", () => {
  it("rotates: the old one dies as the new one is born", async () => {
    const user = await ownerRow();
    const first = await createRefresh(env, user.id);

    const used = await consumeRefresh(env, first);
    expect(used.ok).toBe(true);
    expect(used.adminId).toBe(user.id);

    // The token that was just spent is no longer a token.
    expect((await consumeRefresh(env, first)).ok).toBe(false);
  });

  it("treats a replay as a compromise and ends every session", async () => {
    const user = await ownerRow();
    const first = await createRefresh(env, user.id);
    const second = await createRefresh(env, user.id, first);
    await consumeRefresh(env, first);

    // Someone used the old token after it was rotated: either the thief or the
    // owner, and there is no way to tell which. Everything ends.
    const replay = await consumeRefresh(env, first);
    expect(replay.ok).toBe(false);
    expect(replay.reuse).toBe(true);
    expect((await consumeRefresh(env, second)).ok).toBe(false);
  });

  it("refuses a token nobody issued", async () => {
    expect((await consumeRefresh(env, "made-up")).ok).toBe(false);
  });

  it("can end every session on demand", async () => {
    const user = await ownerRow();
    const token = await createRefresh(env, user.id);
    await endAllSessions(env, user.id);
    expect((await consumeRefresh(env, token)).ok).toBe(false);
  });

  it("writes a cookie a script cannot read and a cross-site form cannot send", () => {
    const cookie = refreshCookie("value", 600);
    expect(cookie).toContain("__Host-shop_refresh=value");
    expect(cookie).toContain("HttpOnly");
    expect(cookie).toContain("Secure");
    expect(cookie).toContain("SameSite=Strict");
    expect(cookie).toContain("Path=/");
  });

  it("reads one cookie out of several", () => {
    const request = new Request(ORIGIN, {
      headers: { cookie: `other=1; ${REFRESH_COOKIE}=wanted; another=2` },
    });
    expect(readCookie(request, REFRESH_COOKIE)).toBe("wanted");
    expect(readCookie(request, "absent")).toBe("");
    expect(readCookie(new Request(ORIGIN), REFRESH_COOKIE)).toBe("");
  });
});

describe("login throttling", () => {
  it("stops a run of failures", async () => {
    expect(await loginAllowed(env, "ip-hash", "a@example.com")).toBe(true);
    for (let i = 0; i < 10; i++) await noteLoginFailure(env, "ip-hash", "a@example.com");
    expect(await loginAllowed(env, "ip-hash", "a@example.com")).toBe(false);

    // A success clears the count, so one forgotten password does not lock
    // somebody out for an hour.
    await clearLoginFailures(env, "ip-hash", "a@example.com");
    expect(await loginAllowed(env, "ip-hash", "a@example.com")).toBe(true);
  });
});

describe("the bootstrap owner", () => {
  it("creates the first owner, once", async () => {
    const withBootstrap = { ...env, SHOP_ADMIN_BOOTSTRAP: "first@example.com:a-long-password" };
    const created = await maybeBootstrap(withBootstrap);
    expect(created?.email_lc).toBe("first@example.com");

    // A second call must not create a second owner, nor reset the first's
    // password back to whatever is still sitting in the environment.
    expect(await maybeBootstrap(withBootstrap)).toBe(null);
    const count = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM admin_users`).first<{ n: number }>();
    expect(count?.n).toBe(1);
  });

  it("does nothing without the variable, or with a malformed one", async () => {
    expect(await maybeBootstrap({ ...env, SHOP_ADMIN_BOOTSTRAP: undefined })).toBe(null);
    expect(await maybeBootstrap({ ...env, SHOP_ADMIN_BOOTSTRAP: "no-colon" })).toBe(null);
    expect(await maybeBootstrap({ ...env, SHOP_ADMIN_BOOTSTRAP: "a@example.com:short" })).toBe(null);
  });
});

describe("the gate", () => {
  it("turns a valid token into an identity", async () => {
    const user = await ownerRow();
    const token = await signJwt(env, user, ORIGIN);
    const request = new Request(`${ORIGIN}/api/shop/admin/me`, {
      headers: { authorization: `Bearer ${token}` },
    });
    const verdict = await requireAdmin(request, env);
    expect(verdict).toMatchObject({ email: user.email_lc, role: "owner", via: "jwt" });
  });

  it("answers 401 without one", async () => {
    const verdict = await requireAdmin(new Request(`${ORIGIN}/api/shop/admin/me`), env);
    expect(verdict).toBeInstanceOf(Response);
    expect((verdict as Response).status).toBe(401);
  });

  it("lets an owner through and keeps staff out of the owner's business", () => {
    const request = new Request(`${ORIGIN}/api/shop/admin/products`, { method: "POST" });
    const owner: AdminIdentity = { sub: "1", email: "o@example.com", role: "owner", via: "jwt" };
    const staff: AdminIdentity = { sub: "2", email: "s@example.com", role: "staff", via: "jwt" };
    expect(requireOwner(request, owner)).toBe(null);
    expect(requireOwner(request, staff)?.status).toBe(403);
  });
});
