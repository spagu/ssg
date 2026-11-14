// Time-based one-time passwords (RFC 6238 over RFC 4226 HOTP), on WebCrypto
// alone. A leading underscore keeps this file out of the Pages route table.
// Pure functions: no database, no clock (the caller passes the time).
//
// What authenticator apps use, and all this worker offers: HMAC-SHA-1, six
// digits, a 30-second step, a 160-bit secret shown as base32. SHA-256/512 and
// eight digits exist in the RFC but most apps ignore those URI parameters, so
// offering them would produce codes that silently never match. The RFC's test
// vectors (8 digits) still run against hotp() with digits = 8 in the tests.

export const STEP_SECONDS = 30;
export const DIGITS = 6;
export const SECRET_BYTES = 20;
// WINDOW: a code from one step before or after the current one is accepted
// (clock drift between the phone and the edge, and the time it takes to type).
export const WINDOW = 1;

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

// base32Encode is RFC 4648 base32 without padding (what otpauth URIs carry).
export function base32Encode(bytes: Uint8Array): string {
  let out = "";
  let bits = 0;
  let value = 0;
  for (const b of bytes) {
    value = (value << 8) | b;
    bits += 8;
    while (bits >= 5) {
      out += ALPHABET[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += ALPHABET[(value << (5 - bits)) & 31];
  return out;
}

// base32Decode reads base32 (case, spaces and padding ignored); null for any
// other character.
export function base32Decode(raw: string): Uint8Array | null {
  const s = raw.toUpperCase().replace(/[\s=]/g, "");
  const out: number[] = [];
  let bits = 0;
  let value = 0;
  for (const c of s) {
    const v = ALPHABET.indexOf(c);
    if (v < 0) return null;
    value = ((value << 5) | v) & 0xffff;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Uint8Array.from(out);
}

export const newSecret = (): Uint8Array => crypto.getRandomValues(new Uint8Array(SECRET_BYTES));

// hotp is RFC 4226: HMAC-SHA-1 over the 8-byte big-endian counter, dynamic
// truncation, the last `digits` decimal digits, zero-padded.
export async function hotp(secret: Uint8Array, counter: number, digits = DIGITS): Promise<string> {
  const key = await crypto.subtle.importKey("raw", secret, { name: "HMAC", hash: "SHA-1" }, false, ["sign"]);
  const msg = new Uint8Array(8);
  new DataView(msg.buffer).setBigUint64(0, BigInt(counter));
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", key, msg));
  const at = mac[mac.length - 1] & 0x0f;
  const bin = ((mac[at] & 0x7f) << 24) | (mac[at + 1] << 16) | (mac[at + 2] << 8) | mac[at + 3];
  return String(bin % 10 ** digits).padStart(digits, "0");
}

export const stepAt = (nowMs: number): number => Math.floor(nowMs / 1000 / STEP_SECONDS);

// normCode keeps the digits a person typed or pasted ("123 456", "123-456");
// null unless exactly DIGITS remain.
export function normCode(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length > 32) return null;
  const d = raw.replace(/[\s-]/g, "");
  return new RegExp(`^\\d{${DIGITS}}$`).test(d) ? d : null;
}

// matchStep returns the time step whose code equals `code` — current, then
// one before, then one after — or null. Every candidate is computed and
// compared, so the time taken does not say which step matched. Replay
// protection (a step at or below the last accepted one is refused) is the
// caller's: it owns the stored step.
export async function matchStep(secret: Uint8Array, code: string, nowMs: number): Promise<number | null> {
  const now = stepAt(nowMs);
  let found: number | null = null;
  for (const step of [now, now - WINDOW, now + WINDOW]) {
    const expected = await hotp(secret, step);
    let diff = 0;
    for (let i = 0; i < DIGITS; i++) diff |= expected.charCodeAt(i) ^ code.charCodeAt(i);
    if (diff === 0 && found === null) found = step;
  }
  return found;
}

// otpauthUri is the Key URI Format authenticator apps read from the QR code:
// otpauth://totp/<issuer>:<account>?secret=…&issuer=…&algorithm=SHA1&digits=6&period=30
export function otpauthUri(issuer: string, account: string, secretB32: string): string {
  const label = `${encodeURIComponent(issuer)}:${encodeURIComponent(account)}`;
  const q = new URLSearchParams({ secret: secretB32, issuer, algorithm: "SHA1", digits: String(DIGITS), period: String(STEP_SECONDS) });
  return `otpauth://totp/${label}?${q.toString()}`;
}

// groupSecret shows a base32 secret in groups of four, easier to copy by hand.
export const groupSecret = (b32: string): string => b32.replace(/(.{4})(?=.)/g, "$1 ");
