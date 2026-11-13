// GO-127 with the send engine: suppressed addresses get no campaign, SMTP
// refusals are classified where the engine records them, soft bounces count
// in a row and reset on an accepted send.
import { test } from "node:test";
import assert from "node:assert/strict";
import { applyBounces, softLimit } from "../src/bounce-store.ts";
import { engineHooks } from "../src/engine-hooks.ts";
import { permanent } from "../src/mail/_mime.ts";
import { findSuppression, suppress } from "../src/suppress.ts";
import { addCampaign, campaignRow, engineEnv, fakeMailer, perPerson, pump, subId, truth } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";

const addr = (i: number) => `${subId(i)}@example.com`;
const softCount = (env: EngineEnv, email: string) =>
  (env.NEWSLETTER_DB.sqlite.prepare("SELECT count FROM soft_bounces WHERE email = ?").get(email) as { count: number } | undefined)?.count ?? 0;

// campaign runs one campaign to the end with a mailer that answers `fail`
// for the listed recipients and accepts everyone else.
async function campaign(env: EngineEnv, failing: string[], detail: string) {
  const id = addCampaign(env.NEWSLETTER_DB);
  const mail = fakeMailer((m) => (failing.includes(m.to) ? permanent(detail) : { ok: true }));
  await pump(env, { open: mail.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  return { id, mail };
}

test("a suppressed address receives no campaign (hard, complaint, manual, erasure)", async () => {
  const env = await engineEnv(6);
  await suppress(env as never, { email: addr(1), reason: "hard", source: "resend", keepEmail: true });
  await suppress(env as never, { email: addr(2), reason: "complaint", source: "mailchannels", keepEmail: true });
  await suppress(env as never, { email: addr(3), reason: "manual", source: "admin", keepEmail: true });
  await suppress(env as never, { email: addr(4), reason: "hard", source: "smtp", keepEmail: false });
  // Soft bounces below the limit are counted, not suppressed: still mailed.
  await applyBounces(env as never, "smtp", [{ id: null, kind: "soft", emails: [addr(5)], source: "resend", detail: "" }]);
  const { id, mail } = await campaign(env, [], "");
  assert.deepEqual([...perPerson(mail.sent).keys()].sort(), [addr(5), addr(6)]);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).total_count, 3, "total counts suppressions that kept the address");
});

test("SMTP: a mailbox refusal suppresses at once; a misconfiguration suppresses nobody", async () => {
  const env = await engineEnv(3);
  await campaign(env, [addr(1)], "smtp 550 at RCPT TO: 5.1.1 <x> user unknown");
  const s = await findSuppression(env as never, addr(1));
  assert.deepEqual([s?.reason, s?.source, s?.email], ["hard", "smtp", addr(1)]);

  // The relay refuses everyone the same way: a sender problem, not the list's.
  for (const detail of ["smtp 550 at RCPT TO: 5.7.1 Relaying denied", "smtp 554 at end of data: 5.7.1 rejected by policy", "smtp 535 at AUTH: bad credentials", "resend returned 422"]) {
    const { id } = await campaign(env, [addr(2), addr(3)], detail);
    assert.deepEqual(truth(env.NEWSLETTER_DB, id), { failed: 2 }, detail);
  }
  assert.equal(await findSuppression(env as never, addr(2)), null);
  assert.equal(await findSuppression(env as never, addr(3)), null);
});

test("soft: three in a row suppress; an accepted send after two resets the count", async () => {
  const env = await engineEnv(2);
  const full = "smtp 552 at RCPT TO: 5.2.2 mailbox full";
  await campaign(env, [addr(1), addr(2)], full);
  await campaign(env, [addr(1), addr(2)], full);
  assert.equal(softCount(env, addr(1)), 2);
  assert.equal(await findSuppression(env as never, addr(1)), null, "below the limit: counted only");
  // addr(2) is accepted this time: its count goes; addr(1) bounces a third time.
  await campaign(env, [addr(1)], full);
  assert.equal(softCount(env, addr(2)), 0);
  const s = await findSuppression(env as never, addr(1));
  assert.deepEqual([s?.reason, s?.count], ["soft", 3]);
  assert.equal(softCount(env, addr(1)), 0, "the counter moved to suppressions");
  // Now addr(1) gets nothing; addr(2) needs three new ones in a row.
  const { mail } = await campaign(env, [addr(2)], full);
  assert.deepEqual([...perPerson(mail.sent).keys()], []);
  assert.equal(softCount(env, addr(2)), 1);
});

test("soft limit is configurable; provider soft counts reset on `delivered`, not on the engine's send", async () => {
  assert.equal(softLimit({} as never), 3);
  const env = await engineEnv(1, { NEWSLETTER_SOFT_BOUNCE_LIMIT: "2" });
  const soft = (id: string) => ({ id, kind: "soft" as const, emails: [addr(1)], source: "resend", detail: "resend: Transient" });
  await applyBounces(env as never, "resend", [soft("e1")]);
  await campaign(env, [], ""); // accepted by the provider: proves nothing about delivery
  assert.equal(softCount(env, addr(1)), 1);
  await applyBounces(env as never, "resend", [{ id: null, kind: "delivered", emails: [addr(1), "not an address"], source: "resend", detail: "" }]);
  assert.equal(softCount(env, addr(1)), 0);
  await applyBounces(env as never, "resend", [soft("e2"), soft("e3")]);
  assert.equal((await findSuppression(env as never, addr(1)))?.reason, "soft");
  // A delivered event with no usable address writes nothing.
  assert.deepEqual(await applyBounces(env as never, "resend", [{ id: null, kind: "delivered", emails: ["nope"], source: "resend", detail: "" }]), { applied: 1, duplicate: 0 });
});

test("precedence: a complaint is never downgraded; an erased address never comes back", async () => {
  const env = await engineEnv(0);
  const a = "a@example.com";
  await applyBounces(env as never, "resend", [{ id: "1", kind: "complaint", emails: [a], source: "resend", detail: "" }]);
  await applyBounces(env as never, "smtp", [{ id: null, kind: "hard", emails: [a], source: "smtp", detail: "" }]);
  await suppress(env as never, { email: a, reason: "manual", source: "erasure", keepEmail: false });
  const s = await findSuppression(env as never, a);
  assert.deepEqual([s?.reason, s?.source, s?.email, s?.count], ["complaint", "resend", null, 3]);
  await applyBounces(env as never, "resend", [{ id: "2", kind: "hard", emails: [a], source: "resend", detail: "" }]);
  assert.equal((await findSuppression(env as never, a))?.email, null);
  // hard outranks manual; equals: the newer wins.
  await suppress(env as never, { email: "b@example.com", reason: "manual", source: "admin", keepEmail: true });
  await applyBounces(env as never, "smtp", [{ id: null, kind: "hard", emails: ["b@example.com"], source: "smtp", detail: "" }]);
  assert.equal((await findSuppression(env as never, "b@example.com"))?.reason, "hard");
});

test("hook_events: duplicates in one report count once; old ids are pruned", async () => {
  const env = await engineEnv(0);
  const b = { id: "same", kind: "hard" as const, emails: ["a@example.com"], source: "resend", detail: "" };
  assert.deepEqual(await applyBounces(env as never, "resend", [b, b]), { applied: 1, duplicate: 1 });
  env.NEWSLETTER_DB.sqlite.prepare("INSERT INTO hook_events (id, received_at) VALUES ('resend:old', '2020-01-01T00:00:00.000Z')").run();
  await applyBounces(env as never, "resend", [{ ...b, id: "new" }]);
  const ids = env.NEWSLETTER_DB.sqlite.prepare("SELECT id FROM hook_events ORDER BY id").all().map((r) => r.id);
  assert.deepEqual(ids, ["resend:new", "resend:same"]);
  assert.deepEqual(await applyBounces(env as never, "resend", []), { applied: 0, duplicate: 0 });
});

test("a failing onDelivered hook does not stop the batch", async (t) => {
  t.mock.method(engineHooks, "onDelivered", async () => {
    throw new Error("hook broke");
  });
  const env = await engineEnv(2);
  const { id } = await campaign(env, [], "");
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 2 });
});
