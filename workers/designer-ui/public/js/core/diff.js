// A line diff of two texts, for showing what a publication changes, what
// someone else changed meanwhile (a conflict) and how a revision differs.
// Pure and generic — no panel-specific import, so editor-ui can copy it as is.
//
// The common head and tail are cut first (an edit usually touches a few
// lines of a long file); the rest is compared by the longest common
// subsequence. A middle too large for that (more than MAX_CELLS line pairs)
// is shown as removed-then-added: correct, just not minimal.

export const MAX_CELLS = 4_000_000;

const linesOf = (s) => (s === "" ? [] : String(s).replace(/\r\n?/g, "\n").split("\n"));

// lineDiff(a, b) → [{op: "same" | "del" | "add", text, a, b}], where a and b
// are the 1-based line numbers in the old and the new text (null on the
// side a line is not in).
export function lineDiff(before, after) {
  const a = linesOf(before);
  const b = linesOf(after);
  let head = 0;
  while (head < a.length && head < b.length && a[head] === b[head]) head++;
  let tail = 0;
  while (tail < a.length - head && tail < b.length - head && a[a.length - 1 - tail] === b[b.length - 1 - tail]) tail++;
  const out = [];
  for (let i = 0; i < head; i++) out.push({ op: "same", text: a[i], a: i + 1, b: i + 1 });
  middle(a.slice(head, a.length - tail), b.slice(head, b.length - tail), head, out);
  for (let i = tail; i > 0; i--) out.push({ op: "same", text: a[a.length - i], a: a.length - i + 1, b: b.length - i + 1 });
  return out;
}

// middle appends the diff of the differing middle parts (offset = head).
function middle(a, b, offset, out) {
  const n = a.length;
  const m = b.length;
  if (n * m > MAX_CELLS) {
    a.forEach((text, i) => out.push({ op: "del", text, a: offset + i + 1, b: null }));
    b.forEach((text, j) => out.push({ op: "add", text, a: null, b: offset + j + 1 }));
    return;
  }
  // lcs[i][j]: length of the LCS of a[i:] and b[j:], one row at a time.
  const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
  }
  let i = 0;
  let j = 0;
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) {
      out.push({ op: "same", text: a[i], a: offset + i + 1, b: offset + j + 1 });
      i++;
      j++;
    } else if (j < m && (i === n || lcs[i][j + 1] >= lcs[i + 1][j])) {
      out.push({ op: "add", text: b[j], a: null, b: offset + j + 1 });
      j++;
    } else {
      out.push({ op: "del", text: a[i], a: offset + i + 1, b: null });
      i++;
    }
  }
}

// stats: how many lines a diff adds and removes.
export function stats(ops) {
  let added = 0;
  let removed = 0;
  for (const o of ops) {
    if (o.op === "add") added++;
    else if (o.op === "del") removed++;
  }
  return { added, removed };
}

// hunks: the changed lines with `context` unchanged lines around each run,
// split where more than twice that many unchanged lines lie between. Each
// hunk is a list of ops; an unchanged text gives no hunk.
export function hunks(ops, context = 3) {
  const changed = ops.map((o, i) => (o.op === "same" ? -1 : i)).filter((i) => i >= 0);
  const out = [];
  let from = -1;
  let to = -1;
  for (const i of changed) {
    if (from >= 0 && i - context <= to + context) {
      to = i;
      continue;
    }
    if (from >= 0) out.push(ops.slice(Math.max(0, from - context), Math.min(ops.length, to + context + 1)));
    from = i;
    to = i;
  }
  if (from >= 0) out.push(ops.slice(Math.max(0, from - context), Math.min(ops.length, to + context + 1)));
  return out;
}
