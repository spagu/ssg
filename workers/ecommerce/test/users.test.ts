// Accounts, roles and passwords.
//
// Most of these are one rule seen from different angles: a shop must not be
// able to lock its owner out of it. The rest are about what a role is for —
// staff answer the email, owners change what the shop charges — and about a
// password never being readable, not even by the person who set it.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import { onRequestGet as listUsers, onRequestPost as createUser } from "../functions/api/shop/admin/users/index";
import { onRequestDelete as deleteUser, onRequestPatch as patchUser } from "../functions/api/shop/admin/users/[id]";
import { onRequestPost as changePassword } from "../functions/api/shop/admin/password";
import { onRequest as adminGate } from "../functions/api/shop/admin/_middleware";
import { onRequestPost as login } from "../functions/api/shop/admin/auth/login";
import { consumeRefresh, createRefresh, signJwt, verifyPassword } from "../functions/api/shop/_auth";
import type { AdminIdentity, AdminUserRow } from "../functions/api/shop/_types";
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedOwner } from "./_helpers";

const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });

interface UserOut {
  id: string;
  email: string;
  name: string | null;
  role: string;
  disabled: boolean;
}
interface UserList {
  users: UserOut[];
  owners: number;
  canManage: boolean;
  you: string;
}

function as(role: "owner" | "staff", sub: string, request: Request, params: Record<string, string> = {}) {
  const admin: AdminIdentity = { sub, email: `${role}@example.com`, role, via: "jwt" };
  return { ...ctx(request, params, { admin }), env: shopEnv() };
}

const body = (method: string, payload: unknown) =>
  new Request(`${ORIGIN}/api/shop/admin/users`, {
    method,
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
  });

async function rowFor(email: string): Promise<AdminUserRow> {
  return (await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE email_lc = ?`)
    .bind(email)
    .first<AdminUserRow>())!;
}

beforeEach(async () => {
  await freshShop();
});

describe("adding people", () => {
  it("creates a colleague as staff by default", async () => {
    const ownerId = await seedOwner();
    const res = (await createUser(
      as("owner", ownerId, body("POST", { email: "Helper@Example.com", name: "A Helper", password: "a-long-enough-password" })) as never,
    )) as Response;

    expect(res.status).toBe(201);
    const { user } = await bodyOf<{ user: UserOut }>(res);
    expect(user).toMatchObject({ email: "helper@example.com", role: "staff", disabled: false });
  });

  it("never hands back a password hash, here or in the list", async () => {
    const ownerId = await seedOwner();
    const created = (await createUser(
      as("owner", ownerId, body("POST", { email: "helper@example.com", password: "a-long-enough-password" })) as never,
    )) as Response;
    expect(await created.text()).not.toMatch(/pbkdf2|pass_hash/);

    const listed = (await listUsers(as("owner", ownerId, get("/api/shop/admin/users")) as never)) as Response;
    const text = await listed.text();
    expect(text).not.toMatch(/pbkdf2|pass_hash/);
    // Reading a hash is the first step to cracking it offline at leisure.
    expect(text).toContain("helper@example.com");
  });

  it("refuses a password short enough to be guessed", async () => {
    const ownerId = await seedOwner();
    const res = (await createUser(
      as("owner", ownerId, body("POST", { email: "helper@example.com", password: "short" })) as never,
    )) as Response;
    expect(res.status).toBe(422);
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "weak_password" });
  });

  it("refuses an address that already signs in, and one that is not an address", async () => {
    const ownerId = await seedOwner("owner@example.com");
    const taken = (await createUser(
      as("owner", ownerId, body("POST", { email: "owner@example.com", password: "a-long-enough-password" })) as never,
    )) as Response;
    expect(taken.status).toBe(409);

    const nonsense = (await createUser(
      as("owner", ownerId, body("POST", { email: "not-an-address", password: "a-long-enough-password" })) as never,
    )) as Response;
    expect(nonsense.status).toBe(422);
  });

  it("keeps staff out of making accounts", async () => {
    const res = (await createUser(
      as("staff", "staff-1", body("POST", { email: "helper@example.com", password: "a-long-enough-password" })) as never,
    )) as Response;
    expect(res.status).toBe(403);
  });
});

describe("the shop keeps an owner", () => {
  it("will not let the only owner stop being one", async () => {
    const ownerId = await seedOwner();
    const res = (await patchUser(
      as("owner", ownerId, body("PATCH", { role: "staff" }), { id: ownerId }) as never,
    )) as Response;
    expect(res.status).toBe(409);
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "last_owner" });
  });

  it("will not let the only owner suspend or delete themselves", async () => {
    const ownerId = await seedOwner();

    const suspended = (await patchUser(
      as("owner", ownerId, body("PATCH", { disabled: true }), { id: ownerId }) as never,
    )) as Response;
    expect(suspended.status).toBe(409);

    const deleted = (await deleteUser(
      as("owner", ownerId, body("DELETE", {}), { id: ownerId }) as never,
    )) as Response;
    expect(deleted.status).toBe(409);
  });

  it("allows all of it once there is a second owner", async () => {
    const first = await seedOwner("first@example.com");
    const second = await seedOwner("second@example.com");

    const res = (await patchUser(
      as("owner", second, body("PATCH", { role: "staff" }), { id: first }) as never,
    )) as Response;
    expect(res.status).toBe(200);
    expect((await rowFor("first@example.com")).role).toBe("staff");
  });

  it("counts a suspended owner as one who cannot sign in", async () => {
    const first = await seedOwner("first@example.com");
    const second = await seedOwner("second@example.com");
    await env.SHOP_DB.prepare(`UPDATE admin_users SET disabled = 1 WHERE id = ?`).bind(second).run();

    // The second owner exists but cannot get in, so the first is still the only
    // way into this shop.
    const res = (await patchUser(
      as("owner", first, body("PATCH", { disabled: true }), { id: first }) as never,
    )) as Response;
    expect(res.status).toBe(409);
  });
});

describe("suspending an account", () => {
  it("signs them out now, not when their token expires", async () => {
    const ownerId = await seedOwner("owner@example.com");
    const staffId = await seedOwner("helper@example.com");
    await env.SHOP_DB.prepare(`UPDATE admin_users SET role = 'staff' WHERE id = ?`).bind(staffId).run();

    const refresh = await createRefresh(env, staffId);
    const user = await rowFor("helper@example.com");
    const token = await signJwt(env, user, ORIGIN);

    await patchUser(as("owner", ownerId, body("PATCH", { disabled: true }), { id: staffId }) as never);

    // The refresh token is gone…
    expect((await consumeRefresh(env, refresh)).ok).toBe(false);

    // …and the access token they are still holding stops working at the gate,
    // rather than lasting out its fifteen minutes.
    const request = new Request(`${ORIGIN}/api/shop/admin/orders`, {
      headers: { authorization: `Bearer ${token}` },
    });
    const res = (await adminGate({
      ...ctx(request),
      env: shopEnv(),
      functionPath: "/api/shop/admin/orders",
    } as never)) as Response;
    expect(res.status).toBe(403);
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "account_disabled" });
  });

  it("refuses their sign-in the same way a wrong password is refused", async () => {
    await seedOwner("helper@example.com", "a-long-enough-password");
    await env.SHOP_DB.prepare(`UPDATE admin_users SET disabled = 1 WHERE email_lc = 'helper@example.com'`).run();

    const suspended = (await login({
      ...ctx(post("/api/shop/admin/auth/login", { email: "helper@example.com", password: "a-long-enough-password" })),
      env: shopEnv(),
    } as never)) as Response;
    const wrong = (await login({
      ...ctx(post("/api/shop/admin/auth/login", { email: "nobody@example.com", password: "whatever-it-was" })),
      env: shopEnv(),
    } as never)) as Response;

    // Whether an address still works here is not something a stranger is told.
    expect(suspended.status).toBe(wrong.status);
    expect(await suspended.text()).toBe(await wrong.text());
  });

  it("keeps what they did in the log after the account is deleted", async () => {
    const ownerId = await seedOwner("owner@example.com");
    const staffId = await seedOwner("helper@example.com");
    await env.SHOP_DB.prepare(`UPDATE admin_users SET role = 'staff' WHERE id = ?`).bind(staffId).run();

    const { audit } = await import("../functions/api/shop/_audit");
    await audit(env, `admin:${staffId}`, "order.resend", "order-1", { number: "O-1" });

    await deleteUser(as("owner", ownerId, body("DELETE", {}), { id: staffId }) as never);

    const entry = await env.SHOP_DB.prepare(`SELECT actor FROM audit_log WHERE action = 'order.resend'`)
      .first<{ actor: string }>();
    // "Who did this" has to keep having an answer after they have gone.
    expect(entry?.actor).toBe(`admin:${staffId}`);
  });
});

describe("changing a password", () => {
  it("takes the current one first", async () => {
    const id = await seedOwner("owner@example.com", "the-current-password");

    const wrong = (await changePassword(
      as("owner", id, post("/api/shop/admin/password", {
        currentPassword: "not-it-at-all",
        newPassword: "a-brand-new-password",
      })) as never,
    )) as Response;
    expect(wrong.status).toBe(403);
    // Still the old one.
    expect(await verifyPassword("the-current-password", (await rowFor("owner@example.com")).pass_hash)).toBe(true);
  });

  it("changes it, and signs every session out", async () => {
    const id = await seedOwner("owner@example.com", "the-current-password");
    const refresh = await createRefresh(env, id);

    const res = (await changePassword(
      as("owner", id, post("/api/shop/admin/password", {
        currentPassword: "the-current-password",
        newPassword: "a-brand-new-password",
      })) as never,
    )) as Response;
    expect(res.status).toBe(200);

    const row = await rowFor("owner@example.com");
    expect(await verifyPassword("a-brand-new-password", row.pass_hash)).toBe(true);
    expect(await verifyPassword("the-current-password", row.pass_hash)).toBe(false);
    // Changing a password because somebody else may have it is pointless if
    // their session survives it.
    expect((await consumeRefresh(env, refresh)).ok).toBe(false);
  });

  it("refuses a short one, and the one already in use", async () => {
    const id = await seedOwner("owner@example.com", "the-current-password");

    for (const payload of [
      { currentPassword: "the-current-password", newPassword: "short" },
      { currentPassword: "the-current-password", newPassword: "the-current-password" },
    ]) {
      const res = (await changePassword(
        as("owner", id, post("/api/shop/admin/password", payload)) as never,
      )) as Response;
      expect(res.status, JSON.stringify(payload)).toBe(422);
    }
  });

  it("does not exist under Cloudflare Access", async () => {
    const id = await seedOwner();
    const res = (await changePassword({
      ...ctx(post("/api/shop/admin/password", { currentPassword: "a", newPassword: "b" }), {}, {
        admin: { sub: id, email: "o@example.com", role: "owner", via: "access" },
      }),
      env: { ...shopEnv(), SHOP_ACCESS_TEAM: "t", SHOP_ACCESS_AUD: "a" },
    } as never)) as Response;
    expect(res.status).toBe(404);
  });

  it("lets an owner reset a colleague's without knowing it", async () => {
    const ownerId = await seedOwner("owner@example.com");
    const staffId = await seedOwner("helper@example.com", "whatever-they-chose");
    const refresh = await createRefresh(env, staffId);

    const res = (await patchUser(
      as("owner", ownerId, body("PATCH", { password: "a-password-they-were-given" }), { id: staffId }) as never,
    )) as Response;
    expect(res.status).toBe(200);

    expect(await verifyPassword("a-password-they-were-given", (await rowFor("helper@example.com")).pass_hash)).toBe(true);
    expect((await consumeRefresh(env, refresh)).ok).toBe(false);
  });
});

describe("the list", () => {
  it("says who you are, how many owners there are, and whether you may change any of it", async () => {
    const ownerId = await seedOwner("owner@example.com");
    await seedOwner("helper@example.com");
    await env.SHOP_DB.prepare(`UPDATE admin_users SET role = 'staff' WHERE email_lc = 'helper@example.com'`).run();

    const asOwner = await bodyOf<UserList>(
      (await listUsers(as("owner", ownerId, get("/api/shop/admin/users")) as never)) as Response,
    );
    expect(asOwner.users).toHaveLength(2);
    expect(asOwner.owners).toBe(1);
    expect(asOwner.you).toBe(ownerId);
    expect(asOwner.canManage).toBe(true);

    // Staff can see that colleagues exist — the audit log names them anyway —
    // and are told they cannot change anything here.
    const asStaff = await bodyOf<UserList>(
      (await listUsers(as("staff", "s-1", get("/api/shop/admin/users")) as never)) as Response,
    );
    expect(asStaff.canManage).toBe(false);
  });

  it("404s for an account that is not there", async () => {
    const ownerId = await seedOwner();
    for (const handler of [patchUser, deleteUser]) {
      const res = (await handler(
        as("owner", ownerId, body("PATCH", { name: "x" }), { id: "no-such-account" }) as never,
      )) as Response;
      expect(res.status).toBe(404);
    }
  });
});

describe("the last corners of the accounts screens", () => {
  it("refuses a body it cannot read", async () => {
    const ownerId = await seedOwner();
    const broken = (method: string) =>
      new Request(`${ORIGIN}/api/shop/admin/users`, {
        method,
        headers: { "content-type": "application/json" },
        body: "{oh no",
      });

    expect(((await createUser(as("owner", ownerId, broken("POST")) as never)) as Response).status).toBe(400);
    expect(
      ((await patchUser(as("owner", ownerId, broken("PATCH"), { id: ownerId }) as never)) as Response).status,
    ).toBe(400);
    expect(
      ((await changePassword(as("owner", ownerId, broken("POST")) as never)) as Response).status,
    ).toBe(400);
  });

  it("refuses a reset that would leave a colleague with a guessable password", async () => {
    const ownerId = await seedOwner("owner@example.com");
    const staffId = await seedOwner("helper@example.com");
    const res = (await patchUser(
      as("owner", ownerId, body("PATCH", { password: "short" }), { id: staffId }) as never,
    )) as Response;
    expect(res.status).toBe(422);
  });

  it("keeps staff out of changing anyone but themselves", async () => {
    const ownerId = await seedOwner();
    for (const handler of [patchUser, deleteUser]) {
      const res = (await handler(
        as("staff", "staff-1", body("PATCH", { role: "owner" }), { id: ownerId }) as never,
      )) as Response;
      expect(res.status).toBe(403);
    }
  });

  it("lets an owner rename someone without touching anything else", async () => {
    const ownerId = await seedOwner("owner@example.com");
    const staffId = await seedOwner("helper@example.com");
    await env.SHOP_DB.prepare(`UPDATE admin_users SET role = 'staff' WHERE id = ?`).bind(staffId).run();

    const res = (await patchUser(
      as("owner", ownerId, body("PATCH", { name: "A Helper" }), { id: staffId }) as never,
    )) as Response;
    expect(res.status).toBe(200);

    const row = await rowFor("helper@example.com");
    expect(row.name).toBe("A Helper");
    expect(row.role).toBe("staff");
    expect(row.disabled).toBe(0);
  });

  it("says nothing changed when nothing did", async () => {
    const ownerId = await seedOwner("owner@example.com");
    await seedOwner("second@example.com");
    const res = (await patchUser(
      as("owner", ownerId, body("PATCH", {}), { id: ownerId }) as never,
    )) as Response;
    expect(res.status).toBe(200);

    const entry = await env.SHOP_DB.prepare(
      `SELECT detail_json FROM audit_log WHERE action = 'admin.update' ORDER BY created_at DESC LIMIT 1`,
    ).first<{ detail_json: string }>();
    expect(JSON.parse(entry!.detail_json).changed).toEqual([]);
  });

  it("records a failed attempt at your own password", async () => {
    const id = await seedOwner("owner@example.com", "the-current-password");
    await changePassword(
      as("owner", id, post("/api/shop/admin/password", {
        currentPassword: "wrong", newPassword: "a-brand-new-password",
      })) as never,
    );
    const entry = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM audit_log WHERE action = 'admin.password_failed'`,
    ).first<{ n: number }>();
    // Somebody trying passwords against a signed-in session is worth a line.
    expect(entry?.n).toBe(1);
  });

  it("answers 404 when the account behind a session has gone", async () => {
    const res = (await changePassword(
      as("owner", "no-such-account", post("/api/shop/admin/password", {
        currentPassword: "whatever-it-was", newPassword: "a-brand-new-password",
      })) as never,
    )) as Response;
    expect(res.status).toBe(404);
  });
});
