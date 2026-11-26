// GO-127: what a refusal or a provider event means for an address.
import { test } from "node:test";
import assert from "node:assert/strict";
import { classifyPermanent, classifyReply, fromMailChannels, fromResend, recipients, senderProblem } from "../src/bounce-classify.ts";

test("SMTP permanent failures: only mailbox refusals suppress", () => {
  const cases: [string, "hard" | "soft" | null][] = [
    ["smtp 550 at RCPT TO: 5.1.1 <a@example.com>: Recipient address rejected: User unknown in relay recipient table", "hard"],
    ["smtp 550 at RCPT TO: mailbox unavailable", "hard"],
    ["smtp 551 at RCPT TO: user not local", "hard"],
    ["smtp 553 at RCPT TO: mailbox name not allowed", "hard"],
    ["smtp 550 at RCPT TO: 5.1.2 bad destination system", "hard"],
    ["smtp 550 at RCPT TO: 5.1.10 null MX", "hard"],
    ["smtp 550 at end of data: 5.1.1 user unknown", "hard"],
    ["smtp 552 at RCPT TO: 5.2.2 mailbox full", "soft"],
    ["smtp 552 at RCPT TO: over quota", "soft"],
    // The sender's problem: never suppress.
    ["smtp 550 at RCPT TO: 5.7.1 Relaying denied", null],
    ["smtp 554 at RCPT TO: 5.7.1 rejected by policy", null],
    ["smtp 550 at RCPT TO: relay access denied", null],
    ["smtp 553 at RCPT TO: sorry, that domain isn't in my list of allowed rcpthosts (#5.7.1)", null],
    ["smtp 550 at RCPT TO: blocked by spam filter", null],
    ["smtp 550 at RCPT TO: 5.1.8 sender address rejected", null],
    ["smtp 550 at end of data: 5.7.26 DMARC policy", null],
    ["smtp 552 at end of data: 5.3.4 message too big", null],
    ["smtp 554 at RCPT TO: transaction failed", null],
    ["smtp 550 at MAIL FROM: 5.1.1 sender unknown", null],
    ["smtp 535 at AUTH: authentication failed", null],
    ["smtp: the server does not offer STARTTLS; refusing to continue in clear text", null],
    ["resend returned 422", null],
    ["mailchannels returned 403", null],
    ["x", null],
  ];
  for (const [detail, want] of cases) assert.equal(classifyPermanent(detail), want, detail);
  assert.equal(classifyReply(450, "4.2.2 mailbox full"), null, "a 4xx is never a bounce here");
});

test("Resend events → classes", () => {
  const ev = (type: string, bounce?: Record<string, unknown>, to: unknown = ["a@example.com"]) =>
    fromResend({ type, data: { to, ...(bounce ? { bounce } : {}) } });
  assert.equal(ev("email.bounced", { type: "Permanent", subType: "General", message: "550 5.1.1 no such user" })?.kind, "hard");
  assert.equal(ev("email.bounced")?.kind, "hard", "email.bounced without detail is a permanent rejection");
  assert.equal(ev("email.bounced", { type: "Transient", subType: "MailboxFull" })?.kind, "soft");
  assert.equal(ev("email.bounced", { type: "Undetermined" })?.kind, "soft");
  assert.equal(ev("email.bounced", { type: "Transient", subType: "MessageTooLarge" }), null);
  assert.equal(ev("email.bounced", { type: "Permanent", subType: "General", message: "550 5.7.1 blocked by policy" }), null);
  const complaint = ev("email.complained", undefined, ["Ann <ann@example.com>"]);
  assert.deepEqual(complaint, { kind: "complaint", emails: ["ann@example.com"], detail: "resend: complaint" });
  assert.equal(ev("email.delivered")?.kind, "delivered");
  for (const t of ["email.sent", "email.opened", "email.clicked", "email.delivery_delayed", undefined]) assert.equal(ev(t as string), null);
  assert.equal(fromResend(null), null);
  assert.deepEqual(fromResend({ type: "email.complained", data: [] })?.emails, []);
});

test("Resend permanent bounces: the server's diagnosticCode decides, provider advice text is not read", () => {
  const ses =
    "The recipient's email provider sent a hard bounce message, but didn't specify the reason for the hard bounce. " +
    "We recommend removing the recipient's email address from your mailing list. Sending messages to addresses that " +
    "produce hard bounces can have a negative impact on your reputation as a sender.";
  const kind = (bounce: Record<string, unknown>) =>
    fromResend({ type: "email.bounced", data: { to: ["a@example.com"], bounce } })?.kind ?? null;
  const general = { type: "Permanent", subType: "General", message: ses };
  assert.equal(kind(general), "hard", "SES General text alone");
  assert.equal(kind({ ...general, diagnosticCode: ["smtp; 550 5.1.1 user unknown"] }), "hard", "array");
  assert.equal(kind({ ...general, diagnosticCode: "smtp; 550 5.1.1 user unknown" }), "hard", "string");
  assert.equal(kind({ ...general, diagnosticCode: ["smtp; 550 5.7.1 blocked by policy"] }), null, "sender problem by code");
  assert.equal(kind({ ...general, diagnosticCode: ["smtp; 550 blocked by policy"] }), null, "sender problem by words");
  assert.equal(kind({ ...general, diagnosticCode: ["smtp; 552 5.2.2 mailbox full"] }), "soft", "soft code in Permanent");
  assert.equal(kind({ ...general, diagnosticCode: ["smtp; 550 mailbox unavailable"] }), "hard", "no code, no sender words");
  for (const diagnosticCode of [undefined, null, 5, [], [1, null], ""]) assert.equal(kind({ ...general, diagnosticCode }), "hard");
  assert.equal(kind({ type: "Permanent", subType: "NoEmail", message: "The mailbox doesn't exist, reputation" }), "hard");
  assert.equal(kind({ type: "Permanent", subType: "Suppressed", message: "on the suppression list, reputation" }), "hard");
  assert.equal(kind({ type: "Permanent", subType: "General", message: "5.7.1 policy" }), null, "code in advice text");
  assert.equal(kind({ type: "Permanent", subType: "Other", message: "blocked by policy" }), null, "unknown subtype keeps words");
  assert.equal(kind({ type: "Permanent", subType: "Other", message: "gone" }), "hard");
  assert.equal(kind({ type: "Permanent", subType: "ContentRejected", diagnosticCode: ["smtp; 550 5.1.1 x"] }), null);
  assert.equal(kind({ type: "Transient", subType: "General", message: "mailbox busy" }), "soft");
  assert.equal(kind({ type: "Transient", subType: "General", message: "blocked by policy" }), null);
});

test("MailChannels events → classes", () => {
  const ev = (event: string, extra: Record<string, unknown> = {}) => fromMailChannels({ event, recipients: ["a@example.com"], ...extra });
  assert.deepEqual(ev("hard-bounced", { status: "550", reason: "5.1.1 user unknown" }), {
    kind: "hard",
    emails: ["a@example.com"],
    detail: "mailchannels: 550 5.1.1 user unknown",
  });
  assert.equal(ev("hard-bounced", { status: 521, reason: "Server does not accept mail" })?.kind, "hard");
  assert.equal(ev("hard-bounced")?.detail, "mailchannels:");
  assert.equal(ev("hard-bounced", { status: "554", reason: "5.7.1 spam detected" }), null);
  assert.equal(ev("complained")?.kind, "complaint");
  assert.equal(ev("delivered")?.kind, "delivered");
  for (const e of ["soft-bounced", "processed", "dropped", "open", "click", "unsubscribed", "test"]) assert.equal(ev(e), null, e);
  assert.deepEqual(fromMailChannels({ event: "complained", recipient: "b@example.com" })?.emails, ["b@example.com"]);
  assert.equal(fromMailChannels("nope"), null);
});

test("recipients and senderProblem", () => {
  assert.deepEqual(recipients(["a@example.com", "", 7, "B <b@example.com>"]), ["a@example.com", "b@example.com"]);
  assert.deepEqual(recipients(undefined), []);
  assert.equal(senderProblem("5.2.2 full"), false);
  assert.equal(senderProblem("5.1.1 unknown"), false);
  assert.equal(senderProblem("5.4.4 unable to route"), true);
  assert.equal(senderProblem("no such user"), false);
  assert.equal(senderProblem("DKIM failed"), true);
});
