// QR code, part two: the symbol and its SVG. A leading underscore keeps this
// file out of the Pages route table. _mfa-qr-code.ts makes the codewords.
//
// qrMatrix lays out the function patterns, places the codewords in the
// standard zig-zag, tries the eight masks and keeps the one with the lowest
// penalty (ISO/IEC 18004 section 7.8.3), then writes the format and version
// information. qrSvg draws it with a four-module quiet zone, dark on white
// whatever the page's theme (scanners expect that), as one path — no style
// attribute, nothing the pages' CSP would refuse.

import { alignmentPositions, chooseVersion, codewords, dataStream, sizeOf } from "./_mfa-qr-code";

export type Matrix = boolean[][]; // [row][column], true = dark

const MASKS: ((x: number, y: number) => boolean)[] = [
  (x, y) => (x + y) % 2 === 0,
  (_x, y) => y % 2 === 0,
  (x) => x % 3 === 0,
  (x, y) => (x + y) % 3 === 0,
  (x, y) => (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0,
  (x, y) => ((x * y) % 2) + ((x * y) % 3) === 0,
  (x, y) => (((x * y) % 2) + ((x * y) % 3)) % 2 === 0,
  (x, y) => (((x + y) % 2) + ((x * y) % 3)) % 2 === 0,
];

const bit = (v: number, i: number): boolean => ((v >>> i) & 1) !== 0;

// bch appends the remainder of value·x^n divided by poly (format and version
// information).
function bch(value: number, n: number, poly: number): number {
  let rem = value;
  for (let i = 0; i < n; i++) rem = (rem << 1) ^ ((rem >>> (n - 1)) * poly);
  return (value << n) | rem;
}

// formatBits: level M (00) and the mask, BCH(15,5), XOR 0x5412.
export const formatBits = (mask: number): number => bch(mask, 10, 0x537) ^ 0x5412;
export const versionBits = (version: number): number => bch(version, 12, 0x1f25);

class Grid {
  size: number;
  dark: Matrix;
  fixed: boolean[][];
  version: number;
  constructor(version: number) {
    this.version = version;
    this.size = sizeOf(version);
    this.dark = Array.from({ length: this.size }, () => new Array<boolean>(this.size).fill(false));
    this.fixed = Array.from({ length: this.size }, () => new Array<boolean>(this.size).fill(false));
  }
  set(x: number, y: number, dark: boolean): void {
    this.dark[y][x] = dark;
    this.fixed[y][x] = true;
  }
  functionPatterns(): void {
    const n = this.size;
    for (let i = 0; i < n; i++) {
      this.set(6, i, i % 2 === 0);
      this.set(i, 6, i % 2 === 0);
    }
    for (const [cx, cy] of [[3, 3], [n - 4, 3], [3, n - 4]]) {
      for (let dy = -4; dy <= 4; dy++) {
        for (let dx = -4; dx <= 4; dx++) {
          const d = Math.max(Math.abs(dx), Math.abs(dy));
          const x = cx + dx;
          const y = cy + dy;
          if (x >= 0 && x < n && y >= 0 && y < n) this.set(x, y, d !== 2 && d !== 4);
        }
      }
    }
    const pos = alignmentPositions(this.version);
    const last = pos.length - 1;
    pos.forEach((cx, i) =>
      pos.forEach((cy, j) => {
        if ((i === 0 && j === 0) || (i === 0 && j === last) || (i === last && j === 0)) return;
        for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) this.set(cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
      }),
    );
    this.format(0); // reserves the area; the real bits come after masking
    if (this.version >= 7) {
      const bits = versionBits(this.version);
      for (let i = 0; i < 18; i++) {
        const a = n - 11 + (i % 3);
        const b = Math.floor(i / 3);
        this.set(a, b, bit(bits, i));
        this.set(b, a, bit(bits, i));
      }
    }
  }
  format(mask: number): void {
    const bits = formatBits(mask);
    const n = this.size;
    for (let i = 0; i <= 5; i++) this.set(8, i, bit(bits, i));
    this.set(8, 7, bit(bits, 6));
    this.set(8, 8, bit(bits, 7));
    this.set(7, 8, bit(bits, 8));
    for (let i = 9; i < 15; i++) this.set(14 - i, 8, bit(bits, i));
    for (let i = 0; i < 8; i++) this.set(n - 1 - i, 8, bit(bits, i));
    for (let i = 8; i < 15; i++) this.set(8, n - 15 + i, bit(bits, i));
    this.set(8, n - 8, true); // the dark module
  }
  place(data: number[]): void {
    const n = this.size;
    let i = 0;
    for (let right = n - 1; right >= 1; right -= 2) {
      if (right === 6) right = 5;
      for (let vert = 0; vert < n; vert++) {
        for (let j = 0; j < 2; j++) {
          const x = right - j;
          const y = ((right + 1) & 2) === 0 ? n - 1 - vert : vert;
          if (!this.fixed[y][x] && i < data.length * 8) {
            this.dark[y][x] = bit(data[i >>> 3], 7 - (i & 7));
            i++;
          }
        }
      }
    }
  }
  mask(m: number): void {
    for (let y = 0; y < this.size; y++) for (let x = 0; x < this.size; x++) if (!this.fixed[y][x] && MASKS[m](x, y)) this.dark[y][x] = !this.dark[y][x];
  }
}

// penalty: rules N1 (runs of five or more), N2 (2×2 blocks), N3 (finder-like
// 1:1:3:1:1 runs with four light modules beside), N4 (dark share away from 50%).
export function penalty(m: Matrix): number {
  const n = m.length;
  const lines = [...m.map((r) => r.map(Number).join("")), ...m.map((_, x) => m.map((r) => Number(r[x])).join(""))];
  let score = 0;
  for (const line of lines) {
    for (const run of line.match(/0{5,}|1{5,}/g) ?? []) score += run.length - 2;
    for (let i = line.indexOf("1011101"); i >= 0; i = line.indexOf("1011101", i + 1)) {
      if (line.slice(Math.max(0, i - 4), i) === "0000" || line.slice(i + 7, i + 11) === "0000") score += 40;
    }
  }
  for (let y = 0; y < n - 1; y++) for (let x = 0; x < n - 1; x++) if (m[y][x] === m[y][x + 1] && m[y][x] === m[y + 1][x] && m[y][x] === m[y + 1][x + 1]) score += 3;
  const dark = m.flat().filter(Boolean).length;
  return score + Math.floor(Math.abs((dark * 20) / (n * n) - 10)) * 10;
}

// qrMatrix encodes text (UTF-8) as a QR code; null when it is too long.
export function qrMatrix(text: string): Matrix | null {
  const bytes = new TextEncoder().encode(text);
  const version = chooseVersion(bytes.length);
  if (version === null) return null;
  const data = codewords(dataStream(bytes, version), version);
  let best: { score: number; dark: Matrix } | null = null;
  for (let m = 0; m < MASKS.length; m++) {
    const s = new Grid(version);
    s.functionPatterns();
    s.place(data);
    s.mask(m);
    s.format(m);
    const score = penalty(s.dark);
    if (!best || score < best.score) best = { score, dark: s.dark };
  }
  return (best as { dark: Matrix }).dark;
}

const escAttr = (s: string): string => s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);

// qrSvg draws the matrix as an accessible image (role="img" with its label).
export function qrSvg(text: string, label: string): string | null {
  const m = qrMatrix(text);
  if (!m) return null;
  const quiet = 4;
  const size = m.length + quiet * 2;
  const path = m.flatMap((row, y) => row.flatMap((d, x) => (d ? [`M${x + quiet} ${y + quiet}h1v1h-1z`] : []))).join("");
  return `<svg class="qr" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${size} ${size}" role="img" aria-label="${escAttr(label)}" shape-rendering="crispEdges"><rect width="${size}" height="${size}" fill="#ffffff"/><path fill="#000000" d="${path}"/></svg>`;
}

