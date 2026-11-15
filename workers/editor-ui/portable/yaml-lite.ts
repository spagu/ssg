// A minimal YAML reader for the top-level keys of .ssg.yaml a panel worker
// reads (editor-ui's site model, designer-ui's settings). Not a YAML parser:
// no npm dependency may come in, and the worker must never guess. It reads block mappings and sequences by
// indentation, one-line scalars and one-line flow collections (yaml-scalar.ts)
// — what a hand-written .ssg.yaml uses. Inside a key it reads, anything else
// (anchors, aliases, tags, merge keys, block scalars | >, values continued
// on the next line, tabs, several documents) throws YamlUnsupported with the
// line; keys it does not need are skipped whatever they contain.

import { YamlUnsupported, inlineValue, stripComment } from "./yaml-scalar";
import type { YamlValue } from "./yaml-scalar";

export { YamlUnsupported };
export type { YamlValue };

interface Line { n: number; indent: number; text: string }

// KEY: "name:" followed by a value, or nothing. Quoted and complex keys are
// beyond this reader.
const KEY = /^([A-Za-z_][A-Za-z0-9_.-]*)[ \t]*:(?:[ \t]+(.*))?$/;
const isItem = (text: string) => text === "-" || text.startsWith("- ");

// meaningful lines: comments and blank lines dropped, comments stripped.
function linesOf(text: string): Line[] {
  const out: Line[] = [];
  text.replace(/^\uFEFF/, "").split(/\r?\n/).forEach((raw, i) => {
    const body = stripComment(raw);
    if (!body.trim()) return;
    const indent = /^ */.exec(body)![0].length;
    if (body[indent] === "\t") throw new YamlUnsupported(i + 1, "a tab used for indentation");
    out.push({ n: i + 1, indent, text: body.slice(indent) });
  });
  return out;
}

class Block {
  i = 0;
  readonly lines: Line[];
  constructor(lines: Line[]) {
    this.lines = lines;
  }
  get cur(): Line | undefined { return this.lines[this.i]; }

  // node reads the block value that starts at the current line.
  node(): YamlValue {
    const at = this.cur!;
    return isItem(at.text) ? this.seq(at.indent) : this.map(at.indent);
  }

  // after "key:" or "-" with nothing on the line: a nested block, or null.
  nested(parent: number, sameIndentSeq: boolean): YamlValue {
    const next = this.cur;
    if (next && next.indent > parent) return this.node();
    if (sameIndentSeq && next && next.indent === parent && isItem(next.text)) return this.seq(parent);
    return null;
  }

  // inline: a value on the key's own line; nothing may continue it below.
  inline(text: string, line: Line): YamlValue {
    const v = inlineValue(text, line.n);
    const next = this.cur;
    if (next && next.indent > line.indent) throw new YamlUnsupported(next.n, "a value continued on the next line");
    return v;
  }

  seq(indent: number): YamlValue[] {
    const out: YamlValue[] = [];
    for (let at = this.cur; at && at.indent === indent && isItem(at.text); at = this.cur) {
      const item = at.text.slice(1).replace(/^ +/, "");
      if (!item) {
        this.i++;
        out.push(this.nested(indent, false));
      } else if (KEY.test(item) || item.startsWith("? ") || item.startsWith("<<")) {
        // "- key: value": a mapping whose keys line up with this first one.
        this.lines[this.i] = { n: at.n, indent: indent + at.text.length - item.length, text: item };
        out.push(this.map(this.lines[this.i].indent));
      } else {
        this.i++;
        out.push(this.inline(item, at));
      }
    }
    this.expectEnd(indent);
    return out;
  }

  map(indent: number): Record<string, YamlValue> {
    const out: Record<string, YamlValue> = {};
    for (let at = this.cur; at && at.indent === indent; at = this.cur) {
      const key = keyOf(at);
      if (Object.hasOwn(out, key.name)) throw new YamlUnsupported(at.n, `the key "${key.name}" twice`);
      this.i++;
      own(out, key.name, key.rest ? this.inline(key.rest, at) : this.nested(indent, true));
    }
    this.expectEnd(indent);
    return out;
  }

  // expectEnd: a block ends where a line is less indented than it.
  expectEnd(indent: number): void {
    const at = this.cur;
    if (at && at.indent > indent) throw new YamlUnsupported(at.n, "indentation this reader cannot follow");
  }
}

// keyOf parses "key: rest"; merge keys and complex keys are refused.
function keyOf(at: Line): { name: string; rest: string } {
  if (at.text.startsWith("<<")) throw new YamlUnsupported(at.n, "a merge key (<<)");
  const m = KEY.exec(at.text);
  if (!m) throw new YamlUnsupported(at.n, "a line this reader cannot follow");
  return { name: m[1], rest: (m[2] ?? "").trim() };
}

// readKeys returns the values of `wanted` top-level keys present in `text`.
// Other top-level keys are skipped with everything indented under them.
export function readKeys(text: string, wanted: ReadonlySet<string>): Record<string, YamlValue> {
  const lines = linesOf(text);
  const out: Record<string, YamlValue> = {};
  let i = 0;
  if (lines[0]?.indent === 0 && /^---$/.test(lines[0].text)) i++;
  while (i < lines.length) {
    const at = lines[i];
    if (at.indent !== 0) throw new YamlUnsupported(at.n, "indentation this reader cannot follow");
    if (/^(?:---|\.\.\.)$/.test(at.text) || at.text.startsWith("%")) throw new YamlUnsupported(at.n, "several YAML documents or a directive");
    let end = i + 1;
    // A top-level key's sequence may start at column 0 ("key:" then "- a").
    while (end < lines.length && (lines[end].indent > 0 || isItem(lines[end].text))) end++;
    const m = KEY.exec(at.text);
    if (m && wanted.has(m[1])) {
      if (Object.hasOwn(out, m[1])) throw new YamlUnsupported(at.n, `the key "${m[1]}" twice`);
      const block = new Block(lines.slice(i + 1, end));
      own(out, m[1], m[2]?.trim() ? block.inline(m[2], at) : block.cur ? block.node() : null);
      const left = block.cur;
      if (left) throw new YamlUnsupported(left.n, "indentation this reader cannot follow");
    } else if (!m) {
      throw new YamlUnsupported(at.n, "a line this reader cannot follow");
    }
    i = end;
  }
  return out;
}

// own sets a key as the object's own data property. A plain assignment of
// "__proto__" would replace the object's prototype instead of storing the key,
// and `in` would see inherited names ("toString") as already present.
function own(out: Record<string, YamlValue>, name: string, value: YamlValue): void {
  Object.defineProperty(out, name, { value, enumerable: true, writable: true, configurable: true });
}
