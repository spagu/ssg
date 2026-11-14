// Password hashing: the stored format, verification, the cost upgrade and the
// rules for a new password.
import { test } from "node:test";
import assert from "node:assert/strict";
import { SCHEME, hashPassword, needsUpgrade, parseHash, passwordProblem, verifyPassword } from "../functions/api/auth/_password.ts";
import { counting } from "./fakes.ts";

test("hashPassword writes the versioned format and verifies", async () => {
  const h = await hashPassword("a good passphrase", 10_000);
  const [scheme, n, salt, hash] = h.split("$");
  assert.equal(scheme, SCHEME);
  assert.equal(n, "10000");
  assert.equal(salt.length, 22);
  assert.equal(hash.length, 43);
  assert.notEqual(h, await hashPassword("a good passphrase", 10_000), "a fresh salt every time");
  assert.equal(await verifyPassword("a good passphrase", h, 10_000, 10_000), true);
  assert.equal(await verifyPassword("a good passphrasf", h, 10_000, 10_000), false);
});

test("parseHash refuses anything but the exact format", () => {
  const good = `${SCHEME}$10000$${"A".repeat(22)}$${"B".repeat(43)}`;
  assert.ok(parseHash(good));
  for (const bad of [
    null,
    "",
    `${SCHEME}$10000$${"A".repeat(22)}`,
    `bcrypt$10000$${"A".repeat(22)}$${"B".repeat(43)}`,
    `${SCHEME}$0$${"A".repeat(22)}$${"B".repeat(43)}`,
    `${SCHEME}$010$${"A".repeat(22)}$${"B".repeat(43)}`,
    `${SCHEME}$12345678901$${"A".repeat(22)}$${"B".repeat(43)}`,
    `${SCHEME}$10000$${"A".repeat(21)}$${"B".repeat(43)}`,
    `${SCHEME}$10000$${"A".repeat(22)}$${"B".repeat(42)}`,
    `${SCHEME}$10000$${"+".repeat(22)}$${"B".repeat(43)}`,
    `${SCHEME}$10000$${"A".repeat(22)}$${"+".repeat(43)}`,
  ]) {
    assert.equal(parseHash(bad), null, String(bad));
  }
});

test("verifyPassword derives exactly once when there is nothing to check", async () => {
  const c = counting();
  try {
    assert.equal(await verifyPassword("whatever it is", null, 20_000, 10_000), false);
    assert.equal(await verifyPassword("whatever it is", "garbage", 20_000, 10_000), false);
    assert.deepEqual(c.calls, [20_000, 20_000], "the dummy runs at the configured cost");
  } finally {
    c.restore();
  }
});

test("verifyPassword: a refused configured cost falls back for the dummy", async () => {
  const c = counting(15_000);
  try {
    assert.equal(await verifyPassword("whatever it is", null, 20_000, 10_000), false);
    assert.deepEqual(c.calls, [20_000, 10_000]);
  } finally {
    c.restore();
  }
});

test("needsUpgrade compares the stored cost with the configured one", async () => {
  const h = await hashPassword("a good passphrase", 10_000);
  assert.equal(needsUpgrade(h, 20_000), true);
  assert.equal(needsUpgrade(h, 10_000), false);
  assert.equal(needsUpgrade(null, 20_000), false);
});

test("passwordProblem: required, length in code points, cap, not the address", () => {
  const rules = { passwordMin: 12, passwordMax: 20 };
  const email = "ada@example.com";
  assert.equal(passwordProblem(undefined, email, rules), "password_required");
  assert.equal(passwordProblem("", email, rules), "password_required");
  assert.equal(passwordProblem("short", email, rules), "password_too_short");
  assert.equal(passwordProblem("😀".repeat(11), email, rules), "password_too_short", "11 characters, 22 UTF-16 units");
  assert.equal(passwordProblem("😀".repeat(12), email, rules), null);
  assert.equal(passwordProblem("x".repeat(21), email, rules), "password_too_long");
  assert.equal(passwordProblem(" ADA@example.com ", email, rules), "password_equals_email");
  assert.equal(passwordProblem("a fine passphrase", email, rules), null);
});
