// A minimal CBOR decoder (RFC 8949) for what WebAuthn sends: the attestation
// object and the COSE public key inside it (GO-140). A leading underscore
// keeps this file out of the Pages route table.
//
// Everything here is attacker-controlled bytes, so the decoder is small and
// strict: unsigned and negative integers up to 2^53, byte and text strings,
// arrays, maps (integer or text keys), false/true/null. Refused with a
// CborError: indefinite lengths, tags, floats, other simple values, a length
// or count beyond the bytes that remain, nesting deeper than MAX_DEPTH, more
// than MAX_ITEMS items in all, a duplicate map key, invalid UTF-8, and input
// over MAX_BYTES. Nothing is allocated before its size is checked against the
// input, so a hostile length cannot make it allocate much.

export const MAX_BYTES = 16 * 1024;
export const MAX_DEPTH = 8;
export const MAX_ITEMS = 512;

export type Cbor = number | Uint8Array | string | boolean | null | Cbor[] | Map<number | string, Cbor>;

export class CborError extends Error {}

const utf8 = new TextDecoder("utf-8", { fatal: true, ignoreBOM: false });

class Reader {
  at = 0;
  items = 0;
  b: Uint8Array;
  constructor(b: Uint8Array) {
    this.b = b;
  }
  need(n: number): void {
    if (n > this.b.length - this.at) throw new CborError("truncated");
  }
  byte(): number {
    this.need(1);
    return this.b[this.at++];
  }
  uint(n: number): number {
    this.need(n);
    let v = 0;
    for (let i = 0; i < n; i++) v = v * 256 + this.b[this.at++];
    if (v > Number.MAX_SAFE_INTEGER) throw new CborError("integer too large");
    return v;
  }
  // arg reads the additional information of a head: the value, or a length.
  arg(info: number): number {
    if (info < 24) return info;
    if (info === 24) return this.uint(1);
    if (info === 25) return this.uint(2);
    if (info === 26) return this.uint(4);
    if (info === 27) return this.uint(8);
    throw new CborError("indefinite length or reserved value");
  }
  take(n: number): Uint8Array {
    this.need(n);
    this.at += n;
    return this.b.slice(this.at - n, this.at);
  }
  value(depth: number): Cbor {
    if (depth > MAX_DEPTH) throw new CborError("nested too deeply");
    if (++this.items > MAX_ITEMS) throw new CborError("too many items");
    const head = this.byte();
    const major = head >> 5;
    const info = head & 31;
    if (major === 7) {
      if (info === 20) return false;
      if (info === 21) return true;
      if (info === 22) return null;
      throw new CborError("unsupported simple value or float");
    }
    if (major === 6) throw new CborError("tags are not supported");
    const n = this.arg(info);
    if (major === 0) return n;
    if (major === 1) return -1 - n;
    if (major === 2) return this.take(n);
    if (major === 3) {
      try {
        return utf8.decode(this.take(n));
      } catch (e) {
        if (e instanceof CborError) throw e;
        throw new CborError("invalid UTF-8");
      }
    }
    // Every item takes at least one byte: a count beyond what remains is a lie.
    this.need(major === 4 ? n : n * 2);
    if (major === 4) return Array.from({ length: n }, () => this.value(depth + 1));
    const map = new Map<number | string, Cbor>();
    for (let i = 0; i < n; i++) {
      const key = this.value(depth + 1);
      if (typeof key !== "number" && typeof key !== "string") throw new CborError("map key must be an integer or text");
      if (map.has(key)) throw new CborError("duplicate map key");
      map.set(key, this.value(depth + 1));
    }
    return map;
  }
}

// decodeFirst decodes one item at the start of `bytes` and says how many bytes
// it took (authenticator data carries a COSE key followed by extensions).
export function decodeFirst(bytes: Uint8Array): { value: Cbor; length: number } {
  if (bytes.length > MAX_BYTES) throw new CborError("input too large");
  const r = new Reader(bytes);
  const value = r.value(0);
  return { value, length: r.at };
}

// decode decodes exactly one item; trailing bytes are an error.
export function decode(bytes: Uint8Array): Cbor {
  const { value, length } = decodeFirst(bytes);
  if (length !== bytes.length) throw new CborError("trailing bytes");
  return value;
}
