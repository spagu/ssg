// POST {base}/hooks/resend        Resend webhook (Svix signature)
// POST {base}/hooks/mailchannels  MailChannels webhook (RFC 9421, Ed25519)
//
// Public routes: app.ts sends /hooks/ past admin auth, CSRF and the per-IP
// limit, because a provider calls from many addresses and without a browser.
// What protects them is the signature, checked here before anything else:
//   secret / key not configured → 503 (never open)
//   body over MAX_BODY          → 413
//   signature or time wrong     → 401, nothing read further, nothing written
// Only then is the body parsed, classified (bounce-classify.ts) and applied
// (bounce-store.ts), with redelivered event ids skipped.

import { fromMailChannels, fromResend } from "./bounce-classify";
import type { Classified } from "./bounce-classify";
import { applyBounces } from "./bounce-store";
import type { Bounce } from "./bounce-store";
import { importEd25519, verifyMessageSignature, verifySvix } from "./hook-verify";
import { json, sha256hex } from "./lib";
import type { RouteContext } from "./router";

export const MAX_BODY = 2 * 1024 * 1024; // MailChannels sends up to 1 000 events per request

const notConfigured = (name: string) => json({ error: "hook_not_configured", detail: `set ${name}` }, 503);
const unauthorized = () => json({ error: "invalid_signature" }, 401);

// readBody returns the raw bytes, or a 413 answer.
async function readBody(request: Request): Promise<Uint8Array | Response> {
  const tooLarge = () => json({ error: "too_large", detail: `at most ${MAX_BODY} bytes` }, 413);
  if (Number(request.headers.get("content-length") ?? 0) > MAX_BODY) return tooLarge();
  const body = new Uint8Array(await request.arrayBuffer());
  return body.length > MAX_BODY ? tooLarge() : body;
}

function parse(body: Uint8Array): unknown {
  try {
    return JSON.parse(new TextDecoder().decode(body));
  } catch {
    return undefined;
  }
}

// toBounce: `delivered` events carry no id — resetting a counter twice is
// harmless, and recording one row per delivered message would cost a write
// per mail sent.
const toBounce = (c: Classified, id: string, source: string): Bounce => ({ id: c.kind === "delivered" ? null : id, source, ...c });

export async function resendHook({ request, env }: RouteContext): Promise<Response> {
  const secret = env.NEWSLETTER_RESEND_WEBHOOK_SECRET;
  if (!secret) return notConfigured("NEWSLETTER_RESEND_WEBHOOK_SECRET");
  const body = await readBody(request);
  if (body instanceof Response) return body;
  if (!(await verifySvix(secret, request.headers, body))) return unauthorized();
  const payload = parse(body);
  if (payload === undefined) return json({ error: "invalid_json" }, 400);
  const c = fromResend(payload);
  // svix-id is the same on every retry of one event: the dedup key.
  const result = await applyBounces(env, "resend", c ? [toBounce(c, request.headers.get("svix-id") as string, "resend")] : []);
  return json({ ok: true, ...result, ignored: c ? 0 : 1 });
}

// eventId: MailChannels events have no id field. The fields that identify an
// event (type, message, request, time, recipients) are hashed instead, so a
// redelivery of the same event has the same id whatever the JSON formatting.
// Only called for an event fromMailChannels classified, i.e. an object.
async function eventId(e: unknown, c: Classified): Promise<string> {
  const o = e as Record<string, unknown>;
  return sha256hex(JSON.stringify([o.event, o.smtp_id, o.request_id, o.timestamp, c.emails]));
}

export async function mailchannelsHook({ request, env }: RouteContext): Promise<Response> {
  const pem = env.NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY;
  if (!pem) return notConfigured("NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY");
  const key = await importEd25519(pem);
  if (!key) {
    console.error("newsletter-ui: NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY is not an Ed25519 public key");
    return notConfigured("NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY to MailChannels' Ed25519 public key (PEM)");
  }
  const body = await readBody(request);
  if (body instanceof Response) return body;
  if (!(await verifyMessageSignature(key, request.headers, body))) return unauthorized();
  const payload = parse(body);
  if (payload === undefined) return json({ error: "invalid_json" }, 400);
  const events = Array.isArray(payload) ? payload : [payload];
  const bounces: Bounce[] = [];
  for (const e of events) {
    const c = fromMailChannels(e);
    if (c) bounces.push(toBounce(c, await eventId(e, c), "mailchannels"));
  }
  const result = await applyBounces(env, "mailchannels", bounces);
  return json({ ok: true, ...result, ignored: events.length - bounces.length });
}
