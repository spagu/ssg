// CSV writing (formula guard, BOM, CRLF) and reading (BOM, CRLF, quotes).
import { test } from "node:test";
import assert from "node:assert/strict";
import { csvCell, decodeCSV, detectDelimiter, parseCSV, toCSV } from "../src/csv.ts";

test("csvCell guards formulas and quotes what needs it", () => {
  for (const f of ["=1+1", "+1", "-1", "@SUM(A1)", "\tx", "\rx"]) assert.ok(csvCell(f).replace(/^"/, "").startsWith("'"), f);
  assert.equal(csvCell('a"b'), '"a""b"');
  assert.equal(csvCell("a,b"), '"a,b"');
  assert.equal(csvCell(" a"), '" a"');
  assert.equal(csvCell(null), "");
  assert.equal(csvCell(undefined), "");
  assert.equal(csvCell(3), "3");
});

test("toCSV: BOM, header, CRLF", () => {
  assert.equal(toCSV(["a", "b"], [{ a: 1, b: "=x" }]), "﻿a,b\r\n1,'=x\r\n");
});

test("parseCSV: BOM, CRLF, LF, CR, quotes, embedded newlines, blank lines", () => {
  const recs = parseCSV('﻿email,tags\r\na@example.com,"x,y"\n\n"b@example.com","say ""hi""\nthere"\rc@example.com,');
  assert.deepEqual(recs, [
    { line: 1, fields: ["email", "tags"] },
    { line: 2, fields: ["a@example.com", "x,y"] },
    { line: 4, fields: ["b@example.com", 'say "hi"\nthere'] },
    { line: 6, fields: ["c@example.com", ""] },
  ]);
  assert.deepEqual(parseCSV(""), []);
  assert.deepEqual(parseCSV("a\n"), [{ line: 1, fields: ["a"] }]);
  assert.deepEqual(parseCSV('"unterminated'), [{ line: 1, fields: ["unterminated"] }]);
  assert.deepEqual(parseCSV('a"b'), [{ line: 1, fields: ['a"b'] }]);
});

test("parseCSV with another delimiter: semicolon and tab, quoted delimiters kept", () => {
  assert.deepEqual(parseCSV('a;"b;c";d,e\n', ";"), [{ line: 1, fields: ["a", "b;c", "d,e"] }]);
  assert.deepEqual(parseCSV("a\t\"b\tc\"\tx,y", "\t"), [{ line: 1, fields: ["a", "b\tc", "x,y"] }]);
});

test("detectDelimiter: most frequent outside quotes on the first line; comma by default", () => {
  assert.equal(detectDelimiter("email;language;tags\na,b,c,d,e"), ";");
  assert.equal(detectDelimiter("email\tname\r\na;b;c"), "\t");
  assert.equal(detectDelimiter('"a;b;c;d",x,y'), ",");
  assert.equal(detectDelimiter('"x"",,,""";a;b'), ";"); // "" inside quotes does not end them
  assert.equal(detectDelimiter("email\na;b"), ",");
  assert.equal(detectDelimiter("a,b;c"), ","); // a tie stays comma
  assert.equal(detectDelimiter("\uFEFFa;b"), ";");
  assert.equal(detectDelimiter(""), ",");
});

test("decodeCSV: UTF-8 (BOM dropped), UTF-16 LE and BE by their BOM", () => {
  const utf8 = new TextEncoder().encode("\uFEFFzoë,ż");
  assert.equal(decodeCSV(utf8), "zoë,ż");
  const le = new Uint8Array([0xff, 0xfe, 0x7a, 0x00, 0x7c, 0x01]); // "zż"
  const be = new Uint8Array([0xfe, 0xff, 0x00, 0x7a, 0x01, 0x7c]);
  assert.equal(decodeCSV(le), "zż");
  assert.equal(decodeCSV(be), "zż");
  assert.equal(decodeCSV(new Uint8Array([0xfe, 0xff, 0x00])), ""); // an odd trailing byte is dropped
  assert.equal(decodeCSV(new Uint8Array()), "");
});
