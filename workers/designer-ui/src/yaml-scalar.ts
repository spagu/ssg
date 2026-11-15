// The one-line half of the minimal YAML reader (yaml-lite.ts): a scalar
// (plain, 'single' or "double" quoted) and a flow collection written on one
// line ([a, b], {k: v}). Anything outside that small subset throws
// YamlUnsupported naming the line — the site model then says so instead of
// guessing (README of the round, GO-169).

export type YamlValue = string | number | boolean | null | YamlValue[] | { [key: string]: YamlValue };

// YamlUnsupported: the file uses something this reader does not understand.
export class YamlUnsupported extends Error {
  readonly line: number;
  readonly reason: string;
  constructor(line: number, reason: string) {
    super(`line ${line}: ${reason}`);
    this.name = "YamlUnsupported";
    this.line = line;
    this.reason = reason;
  }
}

// stripComment cuts a " #" comment that is outside quotes, and trailing space.
export function stripComment(s: string): string {
  let quote = "";
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (quote) {
      if (c === "\\" && quote === '"') i++;
      else if (c === quote) quote = "";
    } else if (c === '"' || c === "'") {
      quote = c;
    } else if (c === "#" && (i === 0 || s[i - 1] === " " || s[i - 1] === "\t")) {
      return s.slice(0, i).trimEnd();
    }
  }
  return s.trimEnd();
}

// typed reads a plain scalar as YAML 1.2's core schema does (what the Go YAML
// library ssg uses reads): booleans, null, integers and floats; else a string.
export function typed(raw: string): YamlValue {
  const s = raw.trim();
  if (/^(?:true|True|TRUE)$/.test(s)) return true;
  if (/^(?:false|False|FALSE)$/.test(s)) return false;
  if (/^(?:null|Null|NULL|~)?$/.test(s)) return null;
  if (/^[-+]?(?:[0-9]+|[0-9]*\.[0-9]+(?:[eE][-+]?[0-9]+)?)$/.test(s)) return Number(s);
  return s;
}

const ESCAPES: Record<string, string> = { "\\": "\\", '"': '"', "/": "/", n: "\n", t: "\t", r: "\r", "0": "\0", " ": " " };

// Cursor walks one line; `line` is its number for error messages.
class Cursor {
  readonly s: string;
  readonly line: number;
  i = 0;
  constructor(s: string, line: number) {
    this.s = s;
    this.line = line;
  }
  space() { while (this.s[this.i] === " ") this.i++; }
  fail(reason: string): never { throw new YamlUnsupported(this.line, reason); }
}

// quoted reads a 'single' or "double" quoted scalar starting at the cursor.
function quoted(c: Cursor): string {
  const q = c.s[c.i++];
  let out = "";
  while (c.i < c.s.length) {
    const ch = c.s[c.i++];
    if (q === "'" && ch === "'") {
      if (c.s[c.i] !== "'") return out;
      out += "'";
      c.i++;
    } else if (q === '"' && ch === '"') {
      return out;
    } else if (q === '"' && ch === "\\") {
      const e = c.s[c.i++];
      const hex = e === "u" ? 4 : e === "x" ? 2 : 0;
      if (hex) {
        const h = c.s.slice(c.i, c.i + hex);
        if (!new RegExp(`^[0-9A-Fa-f]{${hex}}$`).test(h)) c.fail("a malformed escape in a quoted value");
        out += String.fromCharCode(parseInt(h, 16));
        c.i += hex;
      } else if (e in ESCAPES) out += ESCAPES[e];
      else c.fail("an escape this reader does not know");
    } else out += ch;
  }
  return c.fail("a quoted value that continues on the next line");
}

const NODE_PROPERTY = /^[&*!]/;

// flowValue reads one value inside a flow collection (stops at , ] }).
function flowValue(c: Cursor): YamlValue {
  c.space();
  const ch = c.s[c.i];
  if (ch === "[" || ch === "{") return flow(c);
  if (ch === '"' || ch === "'") return quoted(c);
  if (NODE_PROPERTY.test(ch)) c.fail("an anchor, alias or tag");
  const start = c.i;
  while (c.i < c.s.length && !",]}".includes(c.s[c.i]) && !(c.s[c.i] === ":" && c.s[c.i + 1] === " ")) c.i++;
  return typed(c.s.slice(start, c.i));
}

// flow reads [a, b] or {k: v} — on this line only.
function flow(c: Cursor): YamlValue {
  const close = c.s[c.i++] === "[" ? "]" : "}";
  const list: YamlValue[] = [];
  const map: Record<string, YamlValue> = {};
  for (;;) {
    c.space();
    if (c.i >= c.s.length) c.fail("a flow collection that continues on the next line");
    if (c.s[c.i] === close) break;
    const v = flowValue(c);
    c.space();
    if (close === "}") {
      if (c.s[c.i] !== ":") c.fail("a flow mapping entry without a value");
      c.i++;
      const key = String(v);
      if (Object.hasOwn(map, key)) c.fail(`the key "${key}" twice`);
      // Own data (GO-183): an assignment of "__proto__" would set the prototype.
      Object.defineProperty(map, key, { value: flowValue(c), enumerable: true, writable: true, configurable: true });
      c.space();
    } else list.push(v);
    if (c.s[c.i] === ",") c.i++;
    else if (c.s[c.i] !== close) c.fail(c.i >= c.s.length ? "a flow collection that continues on the next line" : "a flow collection this reader cannot follow");
  }
  c.i++;
  return close === "]" ? list : map;
}

// inlineValue reads what follows "key:" or "- " on one line (comment already
// stripped). Block scalars, anchors, aliases and tags are refused.
export function inlineValue(text: string, line: number): YamlValue {
  const c = new Cursor(text.trim(), line);
  const first = c.s.charAt(0);
  if (first === "|" || first === ">") c.fail("a multi-line block value (| or >)");
  if (NODE_PROPERTY.test(first)) c.fail("an anchor, alias or tag");
  if (first !== "[" && first !== "{" && first !== '"' && first !== "'") return typed(c.s);
  const v = first === "[" || first === "{" ? flow(c) : quoted(c);
  c.space();
  if (c.i < c.s.length) c.fail("text after the end of a value");
  return v;
}
