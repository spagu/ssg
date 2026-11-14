// QR code, part one: the codewords (ISO/IEC 18004). A leading underscore
// keeps this file out of the Pages route table. _mfa-qr.ts places them.
//
// Written for one job — an otpauth:// URI on the enrolment page, drawn here
// instead of by an outside service that would then see the secret. So: byte
// mode only, error correction level M (15% of the symbol may be damaged — a
// screen photographed at an angle), versions 1 to 20 (up to 666 bytes; an
// otpauth URI is about 120 to 300). Pure functions.

export const MAX_VERSION = 20;

// Per version (index 1..20) at level M: error-correction codewords per block,
// and the number of blocks (ISO/IEC 18004 Table 9).
const ECC_PER_BLOCK = [0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26];
const BLOCKS = [0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16];

export const sizeOf = (version: number): number => version * 4 + 17;

// alignmentPositions: the row/column centres of the alignment patterns.
export function alignmentPositions(version: number): number[] {
  if (version === 1) return [];
  const count = Math.floor(version / 7) + 2;
  const step = Math.floor((version * 8 + count * 3 + 5) / (count * 4 - 4)) * 2;
  const out = [6];
  for (let pos = sizeOf(version) - 7; out.length < count; pos -= step) out.splice(1, 0, pos);
  return out;
}

// rawModules: the modules left for data and error correction once every
// function pattern (finders, timing, alignment, format and version areas) is
// drawn.
export function rawModules(version: number): number {
  let n = (16 * version + 128) * version + 64;
  if (version >= 2) {
    const align = Math.floor(version / 7) + 2;
    n -= (25 * align - 10) * align - 55;
    if (version >= 7) n -= 36;
  }
  return n;
}

const totalCodewords = (version: number): number => Math.floor(rawModules(version) / 8);
export const dataCodewords = (version: number): number => totalCodewords(version) - ECC_PER_BLOCK[version] * BLOCKS[version];
const countBits = (version: number): number => (version <= 9 ? 8 : 16);

// capacity: the bytes one version holds in byte mode.
export const capacity = (version: number): number => Math.floor((dataCodewords(version) * 8 - 4 - countBits(version)) / 8);

// GF(256) with the QR polynomial x^8 + x^4 + x^3 + x^2 + 1.
export function gfMul(x: number, y: number): number {
  let z = 0;
  for (let i = 7; i >= 0; i--) {
    z = (z << 1) ^ ((z >>> 7) * 0x11d);
    z ^= ((y >>> i) & 1) * x;
  }
  return z;
}

// rsDivisor: the generator polynomial of the given degree, highest term
// implied, coefficients from the second-highest down.
export function rsDivisor(degree: number): number[] {
  const out = new Array<number>(degree).fill(0);
  out[degree - 1] = 1;
  let root = 1;
  for (let i = 0; i < degree; i++) {
    for (let j = 0; j < degree; j++) {
      out[j] = gfMul(out[j], root);
      if (j + 1 < degree) out[j] ^= out[j + 1];
    }
    root = gfMul(root, 0x02);
  }
  return out;
}

// rsRemainder: the error-correction codewords of one block.
export function rsRemainder(data: number[], divisor: number[]): number[] {
  const out = new Array<number>(divisor.length).fill(0);
  for (const b of data) {
    const factor = b ^ (out.shift() as number);
    out.push(0);
    divisor.forEach((coef, i) => (out[i] ^= gfMul(coef, factor)));
  }
  return out;
}

// chooseVersion: the smallest version that holds `length` bytes; null when
// none up to MAX_VERSION does.
export function chooseVersion(length: number): number | null {
  for (let v = 1; v <= MAX_VERSION; v++) if (capacity(v) >= length) return v;
  return null;
}

// dataStream: mode indicator 0100 (byte), the length, the bytes, a terminator
// of up to four zero bits, zero bits to a byte boundary, then the pad bytes
// 0xEC 0x11 alternately up to the version's data capacity.
export function dataStream(bytes: Uint8Array, version: number): number[] {
  const bits: number[] = [];
  const put = (value: number, n: number) => {
    for (let i = n - 1; i >= 0; i--) bits.push((value >>> i) & 1);
  };
  put(0b0100, 4);
  put(bytes.length, countBits(version));
  for (const b of bytes) put(b, 8);
  const room = dataCodewords(version) * 8;
  put(0, Math.min(4, room - bits.length));
  put(0, (8 - (bits.length % 8)) % 8);
  const out: number[] = [];
  for (let i = 0; i < bits.length; i += 8) out.push(bits.slice(i, i + 8).reduce((a, b) => (a << 1) | b, 0));
  for (let pad = 0xec; out.length < dataCodewords(version); pad ^= 0xec ^ 0x11) out.push(pad);
  return out;
}

// codewords: the data split into blocks, each with its error correction, and
// interleaved column by column (short blocks first; they lack the last data
// codeword of the long ones).
export function codewords(data: number[], version: number): number[] {
  const blocks = BLOCKS[version];
  const ecc = ECC_PER_BLOCK[version];
  const raw = totalCodewords(version);
  const shortCount = blocks - (raw % blocks);
  const shortLen = Math.floor(raw / blocks);
  const divisor = rsDivisor(ecc);
  const all: number[][] = [];
  for (let i = 0, k = 0; i < blocks; i++) {
    const dat = data.slice(k, k + shortLen - ecc + (i < shortCount ? 0 : 1));
    k += dat.length;
    const check = rsRemainder(dat, divisor);
    if (i < shortCount) dat.push(0);
    all.push([...dat, ...check]);
  }
  const out: number[] = [];
  for (let i = 0; i < all[0].length; i++) {
    all.forEach((block, j) => {
      if (i !== shortLen - ecc || j >= shortCount) out.push(block[i]);
    });
  }
  return out;
}
