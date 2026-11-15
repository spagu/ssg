// diff.ts: the acceptance table (added, removed, changed, missing final
// newline, CRLF and LF, empty files), the hunks' context, the limits, and a
// randomized check that every script rebuilds the new text with the fewest
// edits (against a plain LCS table).
import { test } from "node:test";
import assert from "node:assert/strict";
import { DIFF_LIMITS, diffLines, diffText, splitLines, toHunks } from "../src/diff.ts";
import type { Run } from "../src/diff.ts";

// compact: a hunk's lines as "+x", "-x", " x" (a trailing "$" = no newline).
const compact = (from: string, to: string) =>
  diffText(from, to).hunks.map((h) => [`@${h.aStart},${h.aLines} ${h.bStart},${h.bLines}`, ...h.lines.map((l) => ({ same: " ", removed: "-", added: "+" })[l.kind] + l.text + (l.noNewline ? "$" : ""))]);

test("the case table", () => {
  const cases: [string, string, string, string[][], number, number][] = [
    ["added line", "a\nb\n", "a\nx\nb\n", [["@1,2 1,3", " a", "+x", " b"]], 1, 0],
    ["removed line", "a\nx\nb\n", "a\nb\n", [["@1,3 1,2", " a", "-x", " b"]], 0, 1],
    ["changed line", "a\nb\nc\n", "a\nB\nc\n", [["@1,3 1,3", " a", "-b", "+B", " c"]], 1, 1],
    ["final newline lost", "a\nb\n", "a\nb", [["@1,2 1,2", " a", "-b", "+b$"]], 1, 1],
    ["final newline added", "a", "a\n", [["@1,1 1,1", "-a$", "+a"]], 1, 1],
    ["empty to text", "", "a\nb\n", [["@0,0 1,2", "+a", "+b"]], 2, 0],
    ["text to empty", "a\n", "", [["@1,1 0,0", "-a"]], 0, 1],
    ["CRLF to LF, one real edit", "a\r\nb\r\nc\r\n", "a\nB\nc\n", [["@1,3 1,3", " a", "-b", "+B", " c"]], 1, 1],
  ];
  for (const [name, from, to, hunks, added, removed] of cases) {
    assert.deepEqual(compact(from, to), hunks, name);
    const d = diffText(from, to);
    assert.deepEqual([d.added, d.removed, d.identical, d.tooLarge], [added, removed, false, null], name);
  }
});

test("only line endings changed: no hunks, not identical, each side's ending said", () => {
  const d = diffText("a\r\nb\r\n", "a\nb\n");
  assert.deepEqual([d.identical, d.hunks, d.from.eol, d.to.eol], [false, [], "crlf", "lf"]);
  assert.deepEqual(diffText("", ""), { identical: true, tooLarge: null, from: { lines: 0, eol: "none", finalNewline: true }, to: { lines: 0, eol: "none", finalNewline: true }, added: 0, removed: 0, hunks: [] });
});

test("splitLines: endings, a lone CR stays in the line, mixed files", () => {
  assert.deepEqual(splitLines("a\r\nb\n"), { lines: ["a", "b"], eol: "mixed", finalNewline: true });
  assert.deepEqual(splitLines("a\rb"), { lines: ["a\rb"], eol: "none", finalNewline: false });
  assert.deepEqual(splitLines("\n"), { lines: [""], eol: "lf", finalNewline: true });
  assert.deepEqual(splitLines("x\r\n"), { lines: ["x"], eol: "crlf", finalNewline: true });
});

test("hunks: three lines of context, close changes share a hunk, far ones do not", () => {
  const base = Array.from({ length: 20 }, (_, i) => `l${i + 1}`);
  const near = base.map((l, i) => (i === 4 || i === 10 ? l.toUpperCase() : l));
  const d = diffText(base.join("\n") + "\n", near.join("\n") + "\n");
  assert.equal(d.hunks.length, 1, "6 unchanged lines between: one hunk");
  assert.deepEqual([d.hunks[0].aStart, d.hunks[0].aLines], [2, 13]);
  const far = base.map((l, i) => (i === 1 || i === 17 ? l.toUpperCase() : l));
  const f = diffText(base.join("\n") + "\n", far.join("\n") + "\n");
  assert.deepEqual(f.hunks.map((h) => [h.aStart, h.aLines, h.bStart, h.bLines]), [[1, 5, 1, 5], [15, 6, 15, 6]]);
  assert.deepEqual(f.hunks[1].lines.at(-1), { kind: "same", text: "l20", a: 20, b: 20 });
  assert.equal(toHunks([{ kind: "same", aStart: 0, bStart: 0, count: 2 }], splitLines("a\nb\n"), splitLines("a\nb\n")).length, 0);
});

test("limits: size, lines and work answer too large with no hunks", () => {
  const big = "x".repeat(DIFF_LIMITS.maxChars + 1);
  assert.equal(diffText(big, "y").tooLarge, "size");
  assert.equal(diffText(big, big).tooLarge, null, "the same text is never too large");
  assert.equal(diffText(big, big).identical, true);
  assert.equal(diffText("a\nb\nc\n", "c\n", { maxLines: 2 }).tooLarge, "lines");
  const w = diffText("a\nb\nc\nd\n", "w\nx\ny\nz\n", { maxWork: 3 });
  assert.deepEqual([w.tooLarge, w.hunks, w.from.lines], ["work", [], 4]);
  assert.equal(diffLines(["a", "b"], ["c", "d"], 1), null);
});

// apply rebuilds b from a and the runs (checks every index on the way).
function apply(a: string[], b: string[], runs: Run[]): string[] {
  const out: string[] = [];
  let i = 0, j = 0;
  for (const r of runs) {
    assert.deepEqual([r.aStart, r.bStart], [i, j]);
    for (let c = 0; c < r.count; c++) {
      if (r.kind === "same") { assert.equal(a[i], b[j]); out.push(a[i]); i++; j++; }
      else if (r.kind === "removed") i++;
      else { out.push(b[j]); j++; }
    }
  }
  assert.deepEqual([i, j], [a.length, b.length]);
  return out;
}
const lcs = (a: string[], b: string[]) => {
  const t = Array.from({ length: a.length + 1 }, () => new Array<number>(b.length + 1).fill(0));
  for (let i = a.length - 1; i >= 0; i--) for (let j = b.length - 1; j >= 0; j--) t[i][j] = a[i] === b[j] ? t[i + 1][j + 1] + 1 : Math.max(t[i + 1][j], t[i][j + 1]);
  return t[0][0];
};

test("random scripts rebuild the new lines with the fewest edits", () => {
  let seed = 7;
  const rnd = (n: number) => ((seed = (seed * 1103515245 + 12345) % 2 ** 31) % n);
  for (let round = 0; round < 400; round++) {
    const a = Array.from({ length: rnd(12) }, () => "abcd"[rnd(4)]);
    const b = Array.from({ length: rnd(12) }, () => "abcd"[rnd(4)]);
    const runs = diffLines(a, b)!;
    assert.deepEqual(apply(a, b, runs), b);
    const edits = runs.reduce((s, r) => s + (r.kind === "same" ? 0 : r.count), 0);
    assert.equal(edits, a.length + b.length - 2 * lcs(a, b), `${a.join("")} → ${b.join("")}`);
  }
});

test("CPU: a 5000-line file with scattered edits stays within the Free plan", () => {
  const a = Array.from({ length: 5000 }, (_, i) => `line ${i} text`);
  const b = a.map((l, i) => (i % 97 === 0 ? l + " changed" : l));
  const from = a.join("\n") + "\n", to = b.join("\n") + "\n";
  const t0 = performance.now();
  const d = diffText(from, to);
  const ms = performance.now() - t0;
  assert.equal(d.tooLarge, null);
  assert.equal(d.added, 52);
  assert.ok(ms < 200, `${ms} ms`); // generous for CI; measured value is in the GO-172 report
});
