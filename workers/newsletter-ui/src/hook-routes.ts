// POST {base}/hooks/resend        Resend webhook (Svix signature)
// POST {base}/hooks/mailchannels  MailChannels webhook (RFC 9421, Ed25519)
//
// Public routes: app.ts sends /hooks/ past admin auth, CSRF and the per-IP
// limit, because a provider calls from many addresses and without a browser.
// What protects them is the signature, checked here before anything else:
//   secret / key not configured → 503 (never open)
//   body over MAX_BODY          → 413
//   signature or time wrong     → 401, nothing read further, nothing written
//   sender domain list invalid  → 503 (hook-origin.ts), nothing written
// An event whose sender is not on NEWSLETTER_HOOK_FROM_DOMAINS (when set) is
// acknowledged as ignored and writes nothing: another site's event.
// Only then is the body parsed, classified (bounce-classify.ts) and settled
// (hook-settle.ts): its arrival counted, bounces applied with redelivered event
// ids skipped, attributed hard bounces shown to the webhook bounce breaker.

import { resendCampaign } from "./bounce-attribution";
import { fromMailChannels, fromResend } from "./bounce-classify";
import type { Classified } from "./bounce-classify";
import { fromUs, hookOrigin } from "./hook-origin";
import type { HookOrigin } from "./hook-origin";
import { importEd25519, verifyMessageSignature, verifySvix } from "./hook-verify";
import { json, sha256hex } from "./lib";
import type { RouteContext } from "./router";
import { settle } from "./hook-settle";
import type { HookEvent } from "./hook-settle";

export const MAX_BODY = 2 * 1024 * 1024; // MailChannels sends up to 1 000 events per request

const notConfigured = (name: string) => json({ error: "hook_not_configured", detail: `set ${name}` }, 503);
const unauthorized = () => json({ error: "invalid_signature" }, 401);
const misconfigured = (origin: HookOrigin) => json({ error: "hook_misconfigured", detail: origin.problem }, 503);
const nothingApplied = (ignored: number) => json({ ok: true, applied: 0, duplicate: 0, ignored });
type Obj = Record<string, unknown>;
const field = (v: unknown, key: string): unknown => (v && typeof v === "object" ? (v as Obj)[key] : undefined);

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

export async function resendHook({ request, env }: RouteContext): Promise<Response> {
  const secret = env.NEWSLETTER_RESEND_WEBHOOK_SECRET;
  if (!secret) return notConfigured("NEWSLETTER_RESEND_WEBHOOK_SECRET");
  const body = await readBody(request);
  if (body instanceof Response) return body;
  if (!(await verifySvix(secret, request.headers, body))) return unauthorized();
  const origin = hookOrigin(env);
  if (origin.problem) return misconfigured(origin);
  const payload = parse(body);
  if (payload === undefined) return json({ error: "invalid_json" }, 400);
  if (!fromUs(origin, field(field(payload, "data"), "from"))) return nothingApplied(1);
  const c = fromResend(payload);
  // svix-id is the same on every retry of one event: the dedup key.
  const event = { classified: c, id: request.headers.get("svix-id") as string, campaign: resendCampaign(payload) };
  const result = await settle(env, "resend", [event]);
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
  const origin = hookOrigin(env);
  if (origin.problem) return misconfigured(origin);
  const payload = parse(body);
  if (payload === undefined) return json({ error: "invalid_json" }, 400);
  const all = Array.isArray(payload) ? payload : [payload];
  // `email` is the sender's address in MailChannels' webhook schema.
  const events = all.filter((e) => fromUs(origin, field(e, "email")));
  // Only other sites' events: nothing to settle, nothing to write.
  if (!events.length && all.length) return nothingApplied(all.length);
  // MailChannels events name no campaign: they stay unattributed (bounce-attribution.ts).
  const settled: HookEvent[] = [];
  for (const e of events) {
    const c = fromMailChannels(e);
    settled.push({ classified: c, id: c ? await eventId(e, c) : "", campaign: null });
  }
  const result = await settle(env, "mailchannels", settled);
  return json({ ok: true, ...result, ignored: all.length - events.length + settled.filter((e) => !e.classified).length });
}
