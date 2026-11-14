// Password hashing: PBKDF2-SHA256 through WebCrypto, the only memory-hard-ish
// option the Workers runtime offers without a dependency. A leading underscore
// keeps this file out of the Pages route table.
//
// Stored format (self-describing, so the cost can be raised without a reset):
//
//   pbkdf2-sha256$<iterations>$<salt, base64url, 16 bytes>$<hash, base64url, 32 bytes>
//
// The algorithm name versions the format: a later scheme gets a new prefix and
// verifyPassword learns it next to this one. needsUpgrade tells the login to
// re-hash with the configured cost after a successful login, while the plain
// password is in hand.

import { b64urlDecode, b64urlEncode, bytesEqual } from "./_lib";

export const SCHEME = "pbkdf2-sha256";
const SALT_BYTES = 16;
const HASH_BYTES = 32;

// kdf is the one place a key is derived; the tests wrap `derive` to count
// calls (the enumeration test asserts the work, not the wall time).
export const kdf = {
  async derive(password: string, salt: Uint8Array, iterations: number): Promise<Uint8Array> {
    const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(password), "PBKDF2", false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt, iterations }, key, HASH_BYTES * 8);
    return new Uint8Array(bits);
  },
};

export interface ParsedHash {
  iterations: number;
  salt: Uint8Array;
  hash: Uint8Array;
}

// parseHash returns null for anything that is not exactly the format above.
export function parseHash(stored: string | null): ParsedHash | null {
  const parts = (stored ?? "").split("$");
  if (parts.length !== 4 || parts[0] !== SCHEME || !/^[1-9][0-9]{0,9}$/.test(parts[1])) return null;
  const salt = b64urlDecode(parts[2]);
  const hash = b64urlDecode(parts[3]);
  if (!salt || salt.length !== SALT_BYTES || !hash || hash.length !== HASH_BYTES) return null;
  return { iterations: Number(parts[1]), salt, hash };
}

export async function hashPassword(password: string, iterations: number): Promise<string> {
  const salt = crypto.getRandomValues(new Uint8Array(SALT_BYTES));
  const hash = await kdf.derive(password, salt, iterations);
  return `${SCHEME}$${iterations}$${b64urlEncode(salt)}$${b64urlEncode(hash)}`;
}

// verifyPassword always derives exactly one key, whether or not there is a
// usable stored hash: an account without one (unknown, provider-only, or a
// malformed row) costs the same work as a wrong password, so the answer time
// does not tell which. The dummy derivation runs at the configured cost; if
// the runtime refuses that cost (AUTH_PBKDF2_ITERATIONS above its cap) it
// falls back to the default rather than turning "no such account" into a
// different error than "wrong password".
export async function verifyPassword(password: string, stored: string | null, iterations: number, fallback: number): Promise<boolean> {
  const parsed = parseHash(stored);
  if (!parsed) {
    const salt = crypto.getRandomValues(new Uint8Array(SALT_BYTES));
    try {
      await kdf.derive(password, salt, iterations);
    } catch {
      await kdf.derive(password, salt, fallback);
    }
    return false;
  }
  return bytesEqual(await kdf.derive(password, parsed.salt, parsed.iterations), parsed.hash);
}

// needsUpgrade: the stored hash is weaker than the configured cost.
export function needsUpgrade(stored: string | null, iterations: number): boolean {
  const parsed = parseHash(stored);
  return parsed !== null && parsed.iterations < iterations;
}

export interface PasswordRules {
  passwordMin: number;
  passwordMax: number;
}

// passwordProblem returns a snake_case reason the new password is refused, or
// null. Length is counted in code points, so a passphrase in any script counts
// what a person typed. The maximum is enforced before any hashing.
export function passwordProblem(password: unknown, email: string, rules: PasswordRules): string | null {
  if (typeof password !== "string" || password === "") return "password_required";
  const length = [...password].length;
  if (length < rules.passwordMin) return "password_too_short";
  if (length > rules.passwordMax) return "password_too_long";
  if (password.trim().toLowerCase() === email) return "password_equals_email";
  return null;
}
