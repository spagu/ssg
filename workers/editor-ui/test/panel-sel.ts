// The selection notation of the editing-rule tests: "|" is the cursor and
// "<|" … "|>" a selection, inside the text they belong to.
export type Sel = { text: string; start: number; end: number };
// at: "a<|b|>c" → {text: "abc", start: 1, end: 2}; "ab|c" → collapsed at 2.
export function at(s: string): Sel {
  if (s.includes("<|")) {
    const start = s.indexOf("<|");
    const t = s.replace("<|", "");
    const end = t.indexOf("|>");
    return { text: t.replace("|>", ""), start, end };
  }
  const i = s.indexOf("|");
  return { text: s.slice(0, i) + s.slice(i + 1), start: i, end: i };
}
// show: the inverse of at, for readable failures.
export const show = (r: Sel | null) => (r === null ? null : r.start === r.end ? r.text.slice(0, r.start) + "|" + r.text.slice(r.start) : r.text.slice(0, r.start) + "<|" + r.text.slice(r.start, r.end) + "|>" + r.text.slice(r.end));
