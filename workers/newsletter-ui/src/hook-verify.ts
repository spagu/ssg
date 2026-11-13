// Webhook signature checks (GO-127). Both run before the body is parsed and
// before anything is written; both refuse a timestamp outside TOLERANCE_S of
// now (either direction), which bounds how long a captured request can be
// replayed (the event-id table catches a replay inside the window).
//
// They read the raw body bytes: a signature is over bytes, and decoding then
// re-encoding text is not guaranteed to give the same ones back.
//
// Comparisons are constant-time: the HMAC and the Ed25519 signature are
// checked by crypto.subtle.verify, the body digest by timingSafeEqual.

export const TOLERANCE_S = 5 * 60; // what Svix and MailChannels document

const enc = new TextEncoder();

export function b64decode(s: string): Uint8Array | null {
  try {
    return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
  } catch {
    return null;
  }
}

const b64encode = (bytes: ArrayBuffer): string => btoa(String.fromCharCode(...new Uint8Array(bytes)));

// timingSafeEqual compares two strings without an early exit.
export function timingSafeEqual(a: string, b: string): boolean {
  const x = enc.encode(a);
  const y = enc.encode(b);
  let diff = x.length ^ y.length;
  for (let i = 0; i < Math.max(x.length, y.length); i++) diff |= (x[i] ?? 0) ^ (y[i] ?? 0);
  return diff === 0;
}

const fresh = (seconds: number, nowMs: number): boolean => Math.abs(nowMs / 1000 - seconds) <= TOLERANCE_S;

// verifySvix checks Resend's signature (the Svix scheme): headers svix-id,
// svix-timestamp (Unix seconds) and svix-signature ("v1,<base64>" entries,
// space-separated — several during a secret rotation); the signature is
// HMAC-SHA256 over "{id}.{timestamp}.{raw body}" keyed with the base64 part of
// the "whsec_…" secret.
export async function verifySvix(secret: string, headers: Headers, body: Uint8Array, nowMs = Date.now()): Promise<boolean> {
  const id = headers.get("svix-id");
  const ts = headers.get("svix-timestamp");
  const sigs = headers.get("svix-signature");
  if (!id || !ts || !sigs || !/^\d{1,12}$/.test(ts) || !fresh(Number(ts), nowMs)) return false;
  const raw = b64decode(secret.startsWith("whsec_") ? secret.slice(6) : secret);
  if (!raw || raw.length === 0) return false;
  const key = await crypto.subtle.importKey("raw", raw, { name: "HMAC", hash: "SHA-256" }, false, ["verify"]);
  const prefix = enc.encode(`${id}.${ts}.`);
  const data = new Uint8Array(prefix.length + body.length);
  data.set(prefix);
  data.set(body, prefix.length);
  for (const entry of sigs.split(" ")) {
    const [version, sig] = entry.split(",", 2);
    const bytes = version === "v1" && sig ? b64decode(sig) : null;
    if (bytes && (await crypto.subtle.verify("HMAC", key, bytes, data))) return true;
  }
  return false;
}

// importEd25519 reads the configured public key: PEM ("-----BEGIN PUBLIC
// KEY-----", SPKI) or bare base64 of the SPKI or of the 32 raw key bytes.
export async function importEd25519(text: string): Promise<CryptoKey | null> {
  const bytes = b64decode(text.replace(/-----[A-Z ]+-----/g, "").replace(/\s+/g, ""));
  if (!bytes) return null;
  try {
    return await crypto.subtle.importKey(bytes.length === 32 ? "raw" : "spki", bytes, { name: "Ed25519" }, false, ["verify"]);
  } catch {
    return null;
  }
}

// One member of Signature-Input: label=("comp" …);created=…;alg="…";keyid="…"
const INPUT_MEMBER = /([A-Za-z0-9_.*-]+)=(\(([^)]*)\)((?:;[A-Za-z0-9_-]+=(?:"[^"]*"|\d+))*))/g;

// verifyMessageSignature checks MailChannels' signature: HTTP Message
// Signatures (RFC 9421) with Ed25519 over the Content-Digest header (RFC 9530,
// sha-256 of the raw body). Accepted only when the signature covers
// content-digest (otherwise the body is not authenticated), covers no derived
// "@" component (not implemented here, so not trusted), names no algorithm
// other than ed25519, and `created` is within the window. The key is the one
// configured, whatever keyid the request names: a request cannot choose the
// key it is checked against.
export async function verifyMessageSignature(key: CryptoKey, headers: Headers, body: Uint8Array, nowMs = Date.now()): Promise<boolean> {
  const input = headers.get("signature-input");
  const signature = headers.get("signature");
  const digest = /(?:^|,)\s*sha-256=:([A-Za-z0-9+/=]+):/.exec(headers.get("content-digest") ?? "")?.[1];
  if (!input || !signature || !digest) return false;
  const actual = b64encode(await crypto.subtle.digest("SHA-256", body));
  if (!timingSafeEqual(actual, digest)) return false;
  for (const m of input.matchAll(INPUT_MEMBER)) {
    const [, label, params, list, rest] = m;
    const components = [...list.matchAll(/"([^"]*)"/g)].map((c) => c[1]);
    const created = /;created=(\d+)/.exec(rest)?.[1];
    const alg = /;alg="([^"]*)"/.exec(rest)?.[1];
    if (!components.includes("content-digest") || components.some((c) => c.startsWith("@"))) continue;
    if ((alg && alg !== "ed25519") || !created || !fresh(Number(created), nowMs)) continue;
    const sig = new RegExp(`(?:^|,)\\s*${label.replace(/[.*-]/g, "\\$&")}=:([A-Za-z0-9+/=]+):`).exec(signature)?.[1];
    const bytes = sig ? b64decode(sig) : null;
    if (!bytes || components.some((c) => headers.get(c) === null)) continue;
    const base = [...components.map((c) => `"${c}": ${headers.get(c)}`), `"@signature-params": ${params}`].join("\n");
    if (await crypto.subtle.verify({ name: "Ed25519" }, key, bytes, enc.encode(base))) return true;
  }
  return false;
}
