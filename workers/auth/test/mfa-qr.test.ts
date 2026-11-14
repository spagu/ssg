// GO-140: the QR encoder, checked by DECODING its output with a decoder written
// here from ISO/IEC 18004 — not from the encoder: the alignment positions and
// the block structure of level M come from the standard's tables (typed in
// below), the format information is matched against all 32 valid codewords,
// and every block's Reed–Solomon syndromes are computed with log/antilog
// tables (the encoder multiplies bit by bit) and must be zero.
import { test } from "node:test";
import assert from "node:assert/strict";
import { penalty, qrMatrix, qrSvg } from "../functions/api/auth/_mfa-qr.ts";
import { capacity } from "../functions/api/auth/_mfa-qr-code.ts";

// ISO/IEC 18004 Annex E: alignment pattern centres, versions 1–20.
const ALIGN: number[][] = [[], [], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34], [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50], [6, 30, 54], [6, 32, 58], [6, 34, 62], [6, 26, 46, 66], [6, 26, 48, 70], [6, 26, 50, 74], [6, 30, 54, 78], [6, 30, 56, 82], [6, 30, 58, 86], [6, 34, 62, 90]];
// ISO/IEC 18004 Table 9, level M: [ec per block, [count, data codewords], …].
const BLOCKS_M: [number, [number, number][]][] = [
  [0, []], [10, [[1, 16]]], [16, [[1, 28]]], [26, [[1, 44]]], [18, [[2, 32]]], [24, [[2, 43]]], [16, [[4, 27]]], [18, [[4, 31]]],
  [22, [[2, 38], [2, 39]]], [22, [[3, 36], [2, 37]]], [26, [[4, 43], [1, 44]]], [30, [[1, 50], [4, 51]]], [22, [[6, 36], [2, 37]]],
  [22, [[8, 37], [1, 38]]], [24, [[4, 40], [5, 41]]], [24, [[5, 41], [5, 42]]], [28, [[7, 45], [3, 46]]], [28, [[10, 46], [1, 47]]],
  [26, [[9, 43], [4, 44]]], [26, [[3, 44], [11, 45]]], [26, [[3, 41], [13, 42]]],
];

// GF(256), primitive polynomial 0x11D, by log/antilog tables.
const EXP = new Array<number>(512);
const LOG = new Array<number>(256);
for (let i = 0, x = 1; i < 255; i++, x = x & 0x80 ? ((x << 1) ^ 0x11d) & 0xff : x << 1) {
  EXP[i] = x;
  LOG[x] = i;
}
for (let i = 255; i < 512; i++) EXP[i] = EXP[i - 255];
const syndromesZero = (cw: number[], ec: number): boolean => {
  for (let k = 0; k < ec; k++) {
    let s = 0;
    for (const c of cw) s = (s === 0 ? 0 : EXP[LOG[s] + k]) ^ c; // Horner at α^k
    if (s !== 0) return false;
  }
  return true;
};

const formatCode = (data: number): number => {
  let r = data << 10;
  for (let i = 14; i >= 10; i--) if ((r >> i) & 1) r ^= 0x537 << (i - 10);
  return ((data << 10) | r) ^ 0x5412;
};

function decode(m: boolean[][]): { text: string; mask: number; version: number } {
  const n = m.length;
  const version = (n - 17) / 4;
  assert.ok(Number.isInteger(version) && version >= 1 && version <= 20, "size");
  const reserved = Array.from({ length: n }, () => new Array<boolean>(n).fill(false));
  const mark = (r: number, c: number) => r >= 0 && c >= 0 && r < n && c < n && (reserved[r][c] = true);
  for (let r = 0; r < 9; r++) for (let c = 0; c < 9; c++) mark(r, c);
  for (let r = 0; r < 9; r++) for (let c = n - 8; c < n; c++) mark(r, c);
  for (let r = n - 8; r < n; r++) for (let c = 0; c < 9; c++) mark(r, c);
  for (let i = 0; i < n; i++) (mark(6, i), mark(i, 6));
  const al = ALIGN[version];
  const inFinder = (r: number, c: number) => (r < 9 && (c < 9 || c > n - 9)) || (r > n - 9 && c < 9);
  for (const r of al) for (const c of al) if (!inFinder(r, c)) for (let dr = -2; dr <= 2; dr++) for (let dc = -2; dc <= 2; dc++) mark(r + dr, c + dc);
  if (version >= 7) for (let i = 0; i < 6; i++) for (let j = n - 11; j < n - 8; j++) (mark(i, j), mark(j, i));
  // Finder look: the three corners hold 7×7 finders.
  for (const [r0, c0] of [[0, 0], [0, n - 7], [n - 7, 0]]) for (let r = 0; r < 7; r++) for (let c = 0; c < 7; c++) {
    const ring = Math.max(Math.abs(r - 3), Math.abs(c - 3));
    assert.equal(m[r0 + r][c0 + c], ring !== 2, `finder at ${r0},${c0}`);
  }
  // Version information (7+), the top-right copy: Annex D values where typed in.
  if (version >= 7) {
    let v = 0;
    for (let i = 17; i >= 0; i--) v = (v << 1) | Number(m[Math.floor(i / 3)][n - 11 + (i % 3)]);
    assert.equal(v >> 12, version, "version information");
    const annexD: Record<number, number> = { 7: 0x07c94, 8: 0x085bc, 9: 0x09a99, 10: 0x0a4d3, 20: 0x149a6 };
    if (annexD[version]) assert.equal(v, annexD[version], `Annex D, version ${version}`);
  }
  // Format information, copy 1 (around the top-left finder) and copy 2.
  const read = (cells: [number, number][]) => cells.reduce((v, [r, c]) => (v << 1) | Number(m[r][c]), 0);
  const copy1 = read([[8, 0], [8, 1], [8, 2], [8, 3], [8, 4], [8, 5], [8, 7], [8, 8], [7, 8], [5, 8], [4, 8], [3, 8], [2, 8], [1, 8], [0, 8]]);
  const copy2 = read([[n - 1, 8], [n - 2, 8], [n - 3, 8], [n - 4, 8], [n - 5, 8], [n - 6, 8], [n - 7, 8], [8, n - 8], [8, n - 7], [8, n - 6], [8, n - 5], [8, n - 4], [8, n - 3], [8, n - 2], [8, n - 1]]);
  assert.equal(copy1, copy2, "both copies of the format information");
  const data = [...Array(32).keys()].find((d) => formatCode(d) === copy1);
  assert.ok(data !== undefined, "a valid format codeword");
  assert.equal(data >> 3, 0b00, "level M");
  const mask = data & 7;
  assert.equal(m[n - 8][8], true, "the dark module");
  const masks = [
    (i: number, j: number) => (i + j) % 2 === 0, (i: number) => i % 2 === 0, (_i: number, j: number) => j % 3 === 0,
    (i: number, j: number) => (i + j) % 3 === 0, (i: number, j: number) => (Math.floor(i / 2) + Math.floor(j / 3)) % 2 === 0,
    (i: number, j: number) => ((i * j) % 2) + ((i * j) % 3) === 0, (i: number, j: number) => (((i * j) % 2) + ((i * j) % 3)) % 2 === 0,
    (i: number, j: number) => (((i * j) % 3) + ((i + j) % 2)) % 2 === 0,
  ];
  const bits: number[] = [];
  let up = true;
  for (let c = n - 1; c > 0; c -= 2) {
    if (c === 6) c--;
    for (let k = 0; k < n; k++) {
      const r = up ? n - 1 - k : k;
      for (const cc of [c, c - 1]) if (!reserved[r][cc]) bits.push(Number(m[r][cc] !== masks[mask](r, cc)));
    }
    up = !up;
  }
  const words: number[] = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) words.push(bits.slice(i, i + 8).reduce((a, b) => (a << 1) | b, 0));
  const [ec, groups] = BLOCKS_M[version];
  const sizes = groups.flatMap(([count, d]) => Array<number>(count).fill(d));
  const blocks = sizes.map(() => [] as number[]);
  let at = 0;
  for (let i = 0; i < Math.max(...sizes); i++) sizes.forEach((d, b) => i < d && blocks[b].push(words[at++]));
  for (let i = 0; i < ec; i++) blocks.forEach((b) => b.push(words[at++]));
  blocks.forEach((b, i) => assert.ok(syndromesZero(b, ec), `block ${i}: Reed–Solomon syndromes`));
  const stream = blocks.flatMap((b, i) => b.slice(0, sizes[i])).flatMap((w) => [7, 6, 5, 4, 3, 2, 1, 0].map((s) => (w >> s) & 1));
  const take = (k: number) => stream.splice(0, k).reduce((a, b) => (a << 1) | b, 0);
  assert.equal(take(4), 0b0100, "byte mode");
  const len = take(version <= 9 ? 8 : 16);
  const bytes = Uint8Array.from({ length: len }, () => take(8));
  return { text: new TextDecoder().decode(bytes), mask, version };
}

const uri = (n: number) => `otpauth://totp/example.com:${"a".repeat(n)}%40example.com?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=example.com&algorithm=SHA1&digits=6&period=30`;

test("QR: what the encoder draws decodes back to the same text, versions 1 to 20, all of level M", () => {
  const versions = new Set<number>();
  const masks = new Set<number>();
  const inputs = ["", "A", "otpauth://x", "héllo — ünïcode", ...[0, 20, 60, 120, 200, 300, 400, 480, 530].map(uri)];
  for (let v = 1; v <= 20; v++) inputs.push("x".repeat(capacity(v)));
  for (const text of inputs) {
    const m = qrMatrix(text) as boolean[][];
    const d = decode(m);
    assert.equal(d.text, text);
    versions.add(d.version);
    masks.add(d.mask);
  }
  assert.deepEqual([...versions].sort((a, b) => a - b), Array.from({ length: 20 }, (_, i) => i + 1));
  assert.ok(masks.size >= 4, `several masks chosen (${[...masks]})`);
});

test("QR: the smallest version that fits; too long for version 20 is refused", () => {
  assert.equal(decode(qrMatrix("x".repeat(14)) as boolean[][]).version, 1);
  assert.equal(decode(qrMatrix("x".repeat(15)) as boolean[][]).version, 2);
  assert.equal(capacity(10), 213, "the standard's byte capacity, 10-M");
  assert.equal(capacity(20), 666, "20-M");
  assert.equal(qrMatrix("x".repeat(667)), null);
  assert.equal(qrSvg("x".repeat(667), "label"), null);
});

test("QR: the penalty prefers the balanced symbol", () => {
  const plain = Array.from({ length: 21 }, () => new Array<boolean>(21).fill(false));
  const checker = Array.from({ length: 21 }, (_, y) => Array.from({ length: 21 }, (_, x) => (x + y) % 2 === 0));
  assert.ok(penalty(plain) > penalty(checker));
  const finder = Array.from({ length: 21 }, () => [false, false, false, false, true, false, true, true, true, false, true, false, false, false, false, ...new Array<boolean>(6).fill(true)]);
  assert.ok(penalty(finder) >= 40 * 21, "finder-like runs cost 40 each");
});

test("QR SVG: one path, a quiet zone, dark on white, labelled, nothing a CSP would refuse", () => {
  const svg = qrSvg(uri(10), 'QR "code" <for> you & me') as string;
  const m = qrMatrix(uri(10)) as boolean[][];
  const size = m.length + 8;
  assert.match(svg, new RegExp(`^<svg class="qr" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${size} ${size}" role="img" aria-label="QR &#34;code&#34; &#60;for&#62; you &#38; me"`));
  assert.equal((svg.match(/<path/g) ?? []).length, 1);
  assert.ok(svg.includes('fill="#ffffff"') && svg.includes('fill="#000000"'));
  assert.ok(!/style=|<script|on\w+=/.test(svg));
  assert.equal((svg.match(/h1v1h-1z/g) ?? []).length, m.flat().filter(Boolean).length, "one square per dark module");
  assert.ok(svg.includes(`M4 4h1v1h-1z`), "the top-left finder's corner, after the four-module quiet zone");
});
