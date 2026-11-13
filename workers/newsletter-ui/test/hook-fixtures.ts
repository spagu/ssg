// Fixtures for the GO-127 tests: providers' webhook requests signed the way
// the providers sign them, and an environment with both hooks configured.
import { handleFetch } from "../src/app.ts";
import { fakeCtx, ORIGIN } from "./fakes.ts";
import { B, readyEnv } from "./subscriber-seed.ts";

const enc = new TextEncoder();
const b64 = (bytes: ArrayBuffer | Uint8Array) => btoa(String.fromCharCode(...new Uint8Array(bytes)));

export const RESEND_SECRET = "whsec_" + b64(new Uint8Array(32).map((_, i) => i * 7 + 1));
const now = () => Math.floor(Date.now() / 1000);

// svixHeaders signs body as Resend (Svix) does.
export async function svixHeaders(body: string, o: { id?: string; ts?: number; secret?: string } = {}): Promise<Record<string, string>> {
  const id = o.id ?? "msg_" + crypto.randomUUID();
  const ts = String(o.ts ?? now());
  const secret = o.secret ?? RESEND_SECRET;
  const key = await crypto.subtle.importKey("raw", Uint8Array.from(atob(secret.slice(6)), (c) => c.charCodeAt(0)), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const sig = await crypto.subtle.sign("HMAC", key, enc.encode(`${id}.${ts}.${body}`));
  return { "svix-id": id, "svix-timestamp": ts, "svix-signature": `v1,${b64(sig)}`, "content-type": "application/json" };
}

// The MailChannels key pair; MC_PEM is what the admin configures.
export const MC_KEYS = (await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"])) as CryptoKeyPair;
const spki = b64(await crypto.subtle.exportKey("spki", MC_KEYS.publicKey));
export const MC_PEM = `-----BEGIN PUBLIC KEY-----\n${spki}\n-----END PUBLIC KEY-----\n`;

// rfc9421Headers signs body as MailChannels does: Content-Digest, then an
// Ed25519 signature over it (RFC 9421).
export async function rfc9421Headers(
  body: string,
  o: { created?: number; key?: CryptoKey; components?: string; alg?: string; digestBody?: string; label?: string } = {},
): Promise<Record<string, string>> {
  const digest = `sha-256=:${b64(await crypto.subtle.digest("SHA-256", enc.encode(o.digestBody ?? body)))}:`;
  const label = o.label ?? "sig_1738775282";
  const params = `(${o.components ?? '"content-digest"'});created=${o.created ?? now()};alg="${o.alg ?? "ed25519"}";keyid="mckey"`;
  const base = `"content-digest": ${digest}\n"@signature-params": ${params}`;
  const sig = await crypto.subtle.sign({ name: "Ed25519" }, o.key ?? MC_KEYS.privateKey, enc.encode(base));
  return { "content-digest": digest, "signature-input": `${label}=${params}`, signature: `${label}=:${b64(sig)}:`, "content-type": "application/json" };
}

export const hookEnv = (overrides: Record<string, unknown> = {}) =>
  readyEnv({ NEWSLETTER_RESEND_WEBHOOK_SECRET: RESEND_SECRET, NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY: MC_PEM, ...overrides });

// hook POSTs to {base}/hooks/<name> with no credentials and no CSRF header,
// as a provider would.
export const hook = (env: unknown, name: string, body: string, headers: Record<string, string>) =>
  handleFetch(new Request(`${ORIGIN}${B}/hooks/${name}`, { method: "POST", body, headers }), env as never, fakeCtx() as never);

export const resendEvent = (type: string, to: string | string[], bounce?: Record<string, unknown>) =>
  JSON.stringify({ type, created_at: "2026-11-13T00:00:00.000Z", data: { email_id: "e1", to: Array.isArray(to) ? to : [to], ...(bounce ? { bounce } : {}) } });

export const mcEvent = (event: string, recipients: string[], extra: Record<string, unknown> = {}) => ({
  email: "news@example.com",
  customer_handle: "acct",
  timestamp: 1763000000,
  event,
  recipients,
  smtp_id: "<m1@example.com>",
  ...extra,
});
