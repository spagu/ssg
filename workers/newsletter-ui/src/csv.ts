// CSV in both directions.
//
// Writing is a copy of the `newsletter` template's _csv.ts (the templates are
// scaffolded independently): a cell that starts with = + - @ tab or CR is
// prefixed with an apostrophe so a spreadsheet shows it as text instead of
// running it as a formula, CRLF line ends (RFC 4180) and a UTF-8 byte-order
// mark so Excel reads non-ASCII text correctly.
//
// Reading is for imports: RFC 4180 with the tolerance real exports need — a
// byte-order mark, CRLF or LF, quoted fields with "" escapes and line breaks,
// UTF-16 files (spreadsheets save "Unicode text" that way) and a semicolon or
// tab instead of the comma (locales whose decimal mark is a comma).

export function csvCell(value: unknown): string {
  let s = value == null ? "" : String(value);
  if (/^[=+\-@\t\r]/.test(s)) s = "'" + s;
  return /[",\r\n]/.test(s) || s !== s.trim() ? `"${s.replaceAll('"', '""')}"` : s;
}

export function toCSV(columns: string[], rows: Record<string, unknown>[]): string {
  const lines = [columns.map(csvCell).join(",")];
  for (const row of rows) lines.push(columns.map((c) => csvCell(row[c])).join(","));
  return "﻿" + lines.join("\r\n") + "\r\n";
}

export interface CSVRecord {
  line: number; // 1-based line the record starts on, for error messages
  fields: string[];
}

export type Delimiter = "," | ";" | "\t";
export const DELIMITERS: readonly Delimiter[] = [",", ";", "\t"];

// decodeCSV turns the raw bytes of an upload into text: UTF-16 when the file
// starts with its byte-order mark (little or big endian), UTF-8 otherwise. Big
// endian is byte-swapped and read as little endian, the one UTF-16 decoder
// every runtime has. The UTF-8 decoder drops a UTF-8 byte-order mark itself.
export function decodeCSV(bytes: Uint8Array): string {
  if (bytes[0] === 0xff && bytes[1] === 0xfe) return new TextDecoder("utf-16le").decode(bytes);
  if (bytes[0] === 0xfe && bytes[1] === 0xff) {
    const swapped = new Uint8Array(bytes.length & ~1); // an odd last byte is half a character: dropped
    for (let i = 0; i + 1 < bytes.length; i += 2) (swapped[i] = bytes[i + 1]), (swapped[i + 1] = bytes[i]);
    return new TextDecoder("utf-16le").decode(swapped);
  }
  return new TextDecoder().decode(bytes);
}

// detectDelimiter picks the separator from the first record (the header line,
// or the first address of a list without one): whichever of comma, semicolon
// and tab occurs most often outside quotes. A tie or none at all means comma,
// so a single-column list reads as one.
export function detectDelimiter(input: string): Delimiter {
  const count: Record<string, number> = { ",": 0, ";": 0, "\t": 0 };
  let quoted = false;
  for (const c of input) {
    if (c === '"') quoted = !quoted; // an escaped "" flips twice: no change
    else if (quoted) continue;
    else if (c === "\n" || c === "\r") break;
    else if (c in count) count[c]++;
  }
  return DELIMITERS.reduce((best, d) => (count[d] > count[best] ? d : best), "," as Delimiter);
}

// parseCSV splits text into records. A record made of one empty field (a blank
// line) is dropped. An unterminated quote runs to the end of the input, as
// spreadsheets read it; the caller's validation then rejects the odd row.
export function parseCSV(input: string, delimiter: Delimiter = ","): CSVRecord[] {
  const s = input.startsWith("﻿") ? input.slice(1) : input;
  const out: CSVRecord[] = [];
  let fields: string[] = [];
  let field = "";
  let quoted = false;
  let line = 1;
  let start = 1;
  const endRecord = () => {
    fields.push(field);
    if (fields.length > 1 || fields[0] !== "") out.push({ line: start, fields });
    fields = [];
    field = "";
  };
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (quoted) {
      if (c === '"') {
        if (s[i + 1] === '"') {
          field += '"';
          i++;
        } else quoted = false;
      } else {
        if (c === "\n") line++;
        field += c;
      }
    } else if (c === '"' && field === "") {
      quoted = true;
    } else if (c === delimiter) {
      fields.push(field);
      field = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && s[i + 1] === "\n") i++;
      endRecord();
      line++;
      start = line;
    } else {
      field += c;
    }
  }
  if (field !== "" || fields.length) endRecord();
  return out;
}
