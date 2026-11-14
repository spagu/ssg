// GO-138: the auth module's summary — counts only, the attention items with
// links into the panel, configuration problems — and its summary kit is the
// shared one, byte for byte.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { summaryAnswer } from "./summary-fixture.ts";
import { req } from "./fakes.ts";
import { mfaEnv } from "./mfa.ts";

test("summary: accounts, invitations, sessions, missing factors; key, factor and invitation attention; mail problem", async () => {
  const res = await summaryAnswer();
  assert.equal(res.status, 200);
  const s = await res.json();
  assert.deepEqual(s.metrics.map((m: { key: string; value: number }) => [m.key, m.value]), [["accounts", 2], ["pendingInvitations", 1], ["liveSessions", 2], ["missingFactor", 1]]);
  assert.deepEqual(s.attention.map((a: { severity: string; href: string }) => [a.severity, a.href]), [["action", "/auth/admin/#/status"], ["warning", "/auth/admin/#/status"], ["info", "/auth/admin/#/invitations"]]);
  assert.match(s.attention[1].text.en, /400 days old/);
  assert.deepEqual(s.problems.map((p: { code: string }) => p.code), ["mail_broken"]);
  assert.ok(!JSON.stringify(s).includes("@example.com"), "aggregates only");
});

test("summary: every configuration problem, and none with a healthy setup", async () => {
  const bad = await (await summaryAnswer({ AUTH_MFA_KEY: "short", AUTH_ACCESS_TEAM: "only-half" })).json();
  assert.deepEqual(bad.problems.map((p: { code: string }) => p.code), ["mail_broken", "mfa_key_short", "access_misconfigured"]);
  // An unreadable policy would also take the admin's own role out of the
  // token, so this one is read from the function itself.
  const { authSummary } = await import("../functions/api/auth/_admin-summary.ts");
  const { prepare } = await import("../functions/api/auth/_context.ts");
  const env = mfaEnv({ AUTH_MFA_ROLES: "nonsense" });
  const err = console.error;
  console.error = () => {};
  const roles = await authSummary((await prepare(req("/api/auth/module/summary"), env)) as never);
  console.error = err;
  assert.deepEqual(roles.problems.map((p) => p.code), ["mfa_roles_invalid"]);
  assert.equal(roles.attention.length, 0);
  const fine = await (await summaryAnswer({ AUTH_MAIL_FROM: "a@example.com" })).json();
  assert.deepEqual(fine.problems, []);
});

test("the summary kit is workers/module-summary-kit.ts, byte for byte", { skip: existsSync(new URL("../../module-summary-kit.ts", import.meta.url)) ? false : "scaffolded alone" }, () => {
  assert.equal(readFileSync(new URL("../functions/api/auth/_summary-kit.ts", import.meta.url), "utf8"), readFileSync(new URL("../../module-summary-kit.ts", import.meta.url), "utf8"), "copy workers/module-summary-kit.ts again");
});

test("the summary kit's own branches: a missing plural form falls back to other; a series is cut to its cap", async () => {
  const { counted, summaryOf, SUMMARY_LIMITS } = await import("../functions/api/auth/_summary-kit.ts");
  const t = { en: "x", pl: "x", hi: "x", de: "x", fr: "x" };
  assert.equal(counted({ en: { other: "{count} n" }, pl: { other: "{count}" }, hi: { other: "{count}" }, de: { other: "{count}" }, fr: { other: "{count}" } }, 1).en, "1 n");
  const points = Array.from({ length: SUMMARY_LIMITS.points + 5 }, (_, i) => ({ day: `2026-01-${i}`, value: i }));
  const s = summaryOf({ metrics: [], series: { label: t, points } });
  assert.equal(s.series?.points.length, SUMMARY_LIMITS.points);
  assert.ok(summaryOf({ metrics: [] }).generatedAt);
});
