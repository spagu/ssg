// Bounce classification (GO-127): what a refusal or a provider event means for
// the address it names.
//
//   hard       the mailbox does not exist → suppressed at once
//   soft       the mailbox exists but could not take the message (full) →
//              counted; suppressed after NEWSLETTER_SOFT_BOUNCE_LIMIT in a row
//   complaint  the recipient reported the mail as spam → suppressed at once
//              and unsubscribed
//   delivered  the provider delivered a message → the soft counter resets
//   null       nothing is done to the address
//
// The rule throughout: when in doubt, return null. A refusal caused by the
// SENDER — a policy or spam block, a missing SPF/DKIM record, failed
// authentication, a relay that does not know us, a message too large — fails
// every recipient the same way. Suppressing on it would wipe the whole list
// the first time the sender is misconfigured, so only codes that are about one
// recipient's mailbox count.

export type BounceKind = "hard" | "soft" | "complaint" | "delivered";

// RFC 3463 enhanced status codes that are about the recipient's mailbox:
//   5.1.1 bad mailbox · 5.1.2 bad domain · 5.1.3 bad address syntax ·
//   5.1.6 mailbox moved · 5.1.10 null MX (the domain accepts no mail).
// 5.1.7 / 5.1.8 are about the SENDER's address and are deliberately absent.
const HARD_ENHANCED = new Set(["5.1.1", "5.1.2", "5.1.3", "5.1.6", "5.1.10"]);
const SOFT_ENHANCED = new Set(["5.2.2"]); // mailbox full
const ENHANCED = /\b([245]\.\d{1,3}\.\d{1,3})\b/;

// Words that mark a refusal as the sender's problem, consulted only when the
// server gave no enhanced code (a code, when present, decides on its own:
// Postfix says "550 5.1.1 … User unknown in relay recipient table").
const SENDER_PROBLEM =
  /\b(policy|spam|block(ed)?|blocklist(ed)?|blacklist(ed)?|reputation|relay(ing)?|auth\w*|dmarc|spf|dkim|rate|too many|content|virus|size|too large)\b/i;

// The SMTP client's permanent failure: "smtp 550 at RCPT TO: 5.1.1 <a@…> unknown".
const SMTP_DETAIL = /^smtp (\d{3}) at ([A-Za-z ]+): ([\s\S]*)$/;
const RECIPIENT_STAGES = new Set(["RCPT TO", "end of data"]);

// classifyReply classifies a 5xx reply text (code + words), from any source.
// `recipientStage` says the reply came where a recipient can be refused.
export function classifyReply(code: number, text: string, recipientStage = true): "hard" | "soft" | null {
  if (code < 500 || code > 599 || !recipientStage) return null;
  const enhanced = ENHANCED.exec(text)?.[1];
  if (enhanced) return HARD_ENHANCED.has(enhanced) ? "hard" : SOFT_ENHANCED.has(enhanced) ? "soft" : null;
  if (SENDER_PROBLEM.test(text)) return null;
  if (code === 550 || code === 551 || code === 553) return "hard";
  return code === 552 ? "soft" : null;
}

// classifyPermanent reads the detail the engine recorded for a permanent
// failure. Only the SMTP client's replies are classified: an HTTP provider's
// synchronous refusal ("resend returned 422") is about the request — a bad
// sender, an unverified domain, a wrong key — never one mailbox; providers
// report bounces through their webhooks instead.
export function classifyPermanent(detail: string): "hard" | "soft" | null {
  const m = SMTP_DETAIL.exec(detail);
  if (!m) return null;
  return classifyReply(Number(m[1]), m[3], RECIPIENT_STAGES.has(m[2]));
}

type Obj = Record<string, unknown>;
const obj = (v: unknown): Obj => (v && typeof v === "object" && !Array.isArray(v) ? (v as Obj) : {});
const str = (v: unknown): string => (typeof v === "string" ? v : "");

// recipients reads an address list: an array of strings or one string, with
// "Name <a@example.com>" reduced to the address.
export function recipients(v: unknown): string[] {
  const list = Array.isArray(v) ? v : [v];
  return list.filter((x): x is string => typeof x === "string" && x.length > 0).map((x) => /<([^<>]+)>\s*$/.exec(x)?.[1] ?? x);
}

export interface Classified {
  kind: BounceKind;
  emails: string[];
  detail: string;
}

// Resend's bounce subtypes that are about the message, not the mailbox.
const RESEND_CONTENT = new Set(["MessageTooLarge", "ContentRejected", "AttachmentRejected"]);

// fromResend maps one Resend webhook payload {type, data: {to, bounce}}.
//   email.bounced     bounce.type Permanent (or absent: Resend sends this event
//                     for permanent rejections) → hard; Transient /
//                     Undetermined → soft; a content subtype or a reply that
//                     reads as the sender's problem → null
//   email.complained  → complaint
//   email.delivered   → delivered
//   anything else     → null (sent, opened, clicked, delivery_delayed: Resend
//                     keeps retrying a delayed message and reports the outcome)
export function fromResend(payload: unknown): Classified | null {
  const p = obj(payload);
  const data = obj(p.data);
  const emails = recipients(data.to);
  switch (p.type) {
    case "email.complained":
      return { kind: "complaint", emails, detail: "resend: complaint" };
    case "email.delivered":
      return { kind: "delivered", emails, detail: "resend: delivered" };
    case "email.bounced": {
      const b = obj(data.bounce);
      const type = str(b.type);
      const detail = `resend: ${type || "bounce"} ${str(b.subType)} ${str(b.message)}`.trim();
      if (RESEND_CONTENT.has(str(b.subType)) || senderProblem(str(b.message))) return null;
      return { kind: type === "Transient" || type === "Undetermined" ? "soft" : "hard", emails, detail };
    }
    default:
      return null;
  }
}

// fromMailChannels maps one MailChannels event {event, recipients, status, reason}.
//   hard-bounced → hard, unless status + reason read as the sender's problem
//   complained   → complaint
//   delivered    → delivered
//   soft-bounced → null: MailChannels retries for hours, may report several
//                  soft-bounced events for ONE message, and sends hard-bounced
//                  when it gives up — counting each would overcount
//   anything else (processed, dropped, open, click, unsubscribed, test) → null
export function fromMailChannels(event: unknown): Classified | null {
  const e = obj(event);
  const emails = recipients(e.recipients ?? e.recipient);
  switch (e.event) {
    case "complained":
      return { kind: "complaint", emails, detail: "mailchannels: complaint" };
    case "delivered":
      return { kind: "delivered", emails, detail: "mailchannels: delivered" };
    case "hard-bounced": {
      const status = String(e.status ?? "");
      const reason = str(e.reason);
      if (senderProblem(`${status} ${reason}`)) return null;
      return { kind: "hard", emails, detail: `mailchannels: ${status} ${reason}`.trim() };
    }
    default:
      return null;
  }
}

// senderProblem: a provider already decided "permanent"; refuse to suppress
// when its words show the refusal was about the sender or the content (an
// enhanced code outside 5.1.x / 5.2.2, or the sender-problem words).
export function senderProblem(text: string): boolean {
  const enhanced = ENHANCED.exec(text)?.[1];
  if (enhanced) return !HARD_ENHANCED.has(enhanced) && !SOFT_ENHANCED.has(enhanced);
  return SENDER_PROBLEM.test(text);
}
