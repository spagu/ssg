// ssg editor-ui — a line diff of two texts, computed in the Worker (no
// dependency). Used by the revisions view (GO-172), and meant for the publish
// dialog's "what will change" and the conflict screen (GO-173). Pure
// functions, no imports: designer-ui may carry a byte-identical copy.
//
// Lines are compared without their line ending: a file whose CRLF became LF
// would otherwise show every line changed, which hides the one real edit —
// each side's `eol` says what the endings are, and `identical` stays false
// when only they differ. A missing final newline is part of the last line,
// as in git: "a" and "a\n" differ on that line (`noNewline` marks it).
//
// The algorithm is Myers' O((N+M)·D) greedy diff after trimming the common
// head and tail. Work is counted and capped, so a huge or wholly rewritten
// file answers "too large to compare" instead of eating the Free plan's
// 10 ms of CPU; memory is O(D²) for the backtrack (bounded by the same cap).

export type Eol = "lf" | "crlf" | "mixed" | "none";
export interface Lines { lines: string[]; eol: Eol; finalNewline: boolean }
export type RunKind = "same" | "removed" | "added";
// Run: `count` consecutive lines of one kind; aStart/bStart are 0-based
// indexes into the old and new lines where the run starts.
export interface Run { kind: RunKind; aStart: number; bStart: number; count: number }
// DiffLine: one line of a hunk; a/b are 1-based line numbers (null on the
// side the line is not on).
export interface DiffLine { kind: RunKind; text: string; a: number | null; b: number | null; noNewline?: true }
export interface Hunk { aStart: number; aLines: number; bStart: number; bLines: number; lines: DiffLine[] }
export type TooLarge = "size" | "lines" | "work";
export interface SideInfo { lines: number; eol: Eol; finalNewline: boolean }
export interface TextDiff {
  identical: boolean; // the texts are the same, character for character
  tooLarge: TooLarge | null; // set → no hunks; offer both versions instead
  from: SideInfo;
  to: SideInfo;
  added: number;
  removed: number;
  hunks: Hunk[];
}
export interface DiffLimits { maxChars: number; maxLines: number; maxWork: number; context: number }

// DIFF_LIMITS: per side 256 Ki characters and 5000 lines; 200 000 steps of
// the algorithm. Measured for GO-172 (Node 25): the worst case — a rewritten
// file hitting the work cap — about 2.3 ms; 5000 lines with 1 % changed
// about 1.6 ms, so the Free plan's 10 ms keeps room for the rest of the
// request. 3 lines of context, as unified diffs.
export const DIFF_LIMITS: DiffLimits = { maxChars: 256 * 1024, maxLines: 5000, maxWork: 200_000, context: 3 };

// splitLines: the lines of a text without their endings. "" has no lines and
// lacks no final newline.
export function splitLines(text: string): Lines {
  if (text === "") return { lines: [], eol: "none", finalNewline: true };
  const lines = text.split("\n");
  let crlf = 0;
  for (let i = 0; i < lines.length - 1; i++) {
    if (lines[i].endsWith("\r")) {
      crlf++;
      lines[i] = lines[i].slice(0, -1);
    }
  }
  const breaks = lines.length - 1;
  const eol: Eol = breaks === 0 ? "none" : crlf === 0 ? "lf" : crlf === breaks ? "crlf" : "mixed";
  const finalNewline = lines[lines.length - 1] === "";
  if (finalNewline) lines.pop();
  return { lines, eol, finalNewline };
}

// diffLines: the runs turning `a` into `b`, or null when the work cap is hit.
export function diffLines(a: readonly string[], b: readonly string[], maxWork = DIFF_LIMITS.maxWork): Run[] | null {
  const ids = new Map<string, number>();
  const intern = (s: string) => {
    let id = ids.get(s);
    if (id === undefined) ids.set(s, (id = ids.size));
    return id;
  };
  const A = Int32Array.from(a, intern);
  const B = Int32Array.from(b, intern);
  let head = 0;
  while (head < A.length && head < B.length && A[head] === B[head]) head++;
  let tail = 0;
  while (tail < A.length - head && tail < B.length - head && A[A.length - 1 - tail] === B[B.length - 1 - tail]) tail++;
  const middle = myers(A.subarray(head, A.length - tail), B.subarray(head, B.length - tail), maxWork);
  if (!middle) return null;
  const runs: Run[] = [];
  const push = (kind: RunKind, aStart: number, bStart: number) => {
    const last = runs[runs.length - 1];
    if (last && last.kind === kind) last.count++;
    else runs.push({ kind, aStart, bStart, count: 1 });
  };
  for (let i = 0; i < head; i++) push("same", i, i);
  for (const [kind, x, y] of middle) push(kind, x + head, y + head);
  for (let i = 0; i < tail; i++) push("same", A.length - tail + i, B.length - tail + i);
  return runs;
}

type Step = [RunKind, number, number];

// myers: the edit script of a → b as steps [kind, aIndex, bIndex], in order.
function myers(a: Int32Array, b: Int32Array, maxWork: number): Step[] | null {
  const n = a.length, m = b.length, max = n + m, off = max + 1;
  const v = new Int32Array(2 * max + 3);
  const trace: Int32Array[] = [];
  let work = 0;
  // No upper bound on d is needed: at d = n + m the end is always reached.
  for (let d = 0, end = false; !end; d++) {
    trace.push(v.slice(off - d - 1, off + d + 2)); // v before step d, k in [-d-1, d+1]
    for (let k = -d; k <= d && !end; k += 2) {
      let x = k === -d || (k !== d && v[off + k - 1] < v[off + k + 1]) ? v[off + k + 1] : v[off + k - 1] + 1;
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) { x++; y++; work++; }
      v[off + k] = x;
      end = x >= n && y >= m;
      if (!end && ++work > maxWork) return null;
    }
  }
  return backtrack(trace, n, m);
}

function backtrack(trace: Int32Array[], n: number, m: number): Step[] {
  const out: Step[] = [];
  let x = n, y = m;
  for (let d = trace.length - 1; d >= 0; d--) {
    const t = trace[d], at = (k: number) => t[k + d + 1];
    const k = x - y;
    const prevK = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
    const prevX = d === 0 ? 0 : at(prevK), prevY = d === 0 ? 0 : prevX - prevK;
    while (x > prevX && y > prevY) { x--; y--; out.push(["same", x, y]); }
    if (d === 0) break;
    if (x === prevX) out.push(["added", x, y - 1]);
    else out.push(["removed", x - 1, y]);
    x = prevX;
    y = prevY;
  }
  return out.reverse();
}

// toHunks groups the changes with `context` unchanged lines around them;
// changes closer than twice the context share a hunk (as unified diffs do).
export function toHunks(runs: readonly Run[], a: Lines, b: Lines, context = DIFF_LIMITS.context): Hunk[] {
  const hunks: Hunk[] = [];
  const line = (kind: RunKind, i: number, j: number): DiffLine => {
    const src = kind === "added" ? b : a, idx = kind === "added" ? j : i;
    const out: DiffLine = { kind, text: src.lines[idx], a: kind === "added" ? null : i + 1, b: kind === "removed" ? null : j + 1 };
    if (!src.finalNewline && idx === src.lines.length - 1) out.noNewline = true;
    return out;
  };
  let cur: Hunk | null = null;
  runs.forEach((r, ri) => {
    if (r.kind !== "same") {
      if (!cur) {
        const lead = Math.min(context, r.aStart, r.bStart);
        cur = { aStart: r.aStart - lead + 1, aLines: 0, bStart: r.bStart - lead + 1, bLines: 0, lines: [] };
        for (let i = lead; i > 0; i--) cur.lines.push(line("same", r.aStart - i, r.bStart - i));
        hunks.push(cur);
      }
      for (let i = 0; i < r.count; i++) cur.lines.push(r.kind === "removed" ? line("removed", r.aStart + i, -1) : line("added", -1, r.bStart + i));
      return;
    }
    if (!cur) return;
    const last = ri === runs.length - 1;
    const keep = !last && r.count <= 2 * context ? r.count : Math.min(context, r.count);
    for (let i = 0; i < keep; i++) cur.lines.push(line("same", r.aStart + i, r.bStart + i));
    if (keep < r.count || last) cur = null;
  });
  for (const h of hunks) {
    h.aLines = h.lines.filter((l) => l.kind !== "added").length;
    h.bLines = h.lines.filter((l) => l.kind !== "removed").length;
    if (h.aLines === 0) h.aStart--; // unified-diff convention: an empty side starts before its line
    if (h.bLines === 0) h.bStart--;
  }
  return hunks;
}

// keys: what a line is compared by — its text, and for a last line without
// a final newline a "\n" in front (no line contains one, so it never matches).
const keys = (l: Lines) => (l.finalNewline ? l.lines : l.lines.map((s, i) => (i === l.lines.length - 1 ? "\n" + s : s)));
const info = (l: Lines): SideInfo => ({ lines: l.lines.length, eol: l.eol, finalNewline: l.finalNewline });

// diffText: the whole comparison of two texts, within the limits.
export function diffText(from: string, to: string, limits: Partial<DiffLimits> = {}): TextDiff {
  const lim = { ...DIFF_LIMITS, ...limits };
  const empty = (tooLarge: TooLarge | null, a: SideInfo, b: SideInfo): TextDiff => ({ identical: from === to, tooLarge, from: a, to: b, added: 0, removed: 0, hunks: [] });
  const unknown: SideInfo = { lines: 0, eol: "none", finalNewline: true };
  if (from.length > lim.maxChars || to.length > lim.maxChars) return empty(from === to ? null : "size", unknown, unknown);
  const a = splitLines(from), b = splitLines(to);
  if (from === to) return empty(null, info(a), info(b));
  if (a.lines.length > lim.maxLines || b.lines.length > lim.maxLines) return empty("lines", info(a), info(b));
  const runs = diffLines(keys(a), keys(b), lim.maxWork);
  if (!runs) return empty("work", info(a), info(b));
  const count = (kind: RunKind) => runs.reduce((s, r) => s + (r.kind === kind ? r.count : 0), 0);
  return { identical: false, tooLarge: null, from: info(a), to: info(b), added: count("added"), removed: count("removed"), hunks: toHunks(runs, a, b, lim.context) };
}
