// Column detection: the alias table over realistic export layouts, priority
// on ambiguity, names reported and never used, the explicit override.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ALIASES, NAMES_NOT_STORED, NOT_RECOGNISED, ROLES, detectColumns, normaliseHeader, parseMapping } from "../src/import-columns.ts";
import { readRows } from "../src/import-plan.ts";
import { LAYOUTS } from "./import-fixtures.ts";

test("normaliseHeader ignores case, spaces, underscores, hyphens and dots", () => {
  for (const h of ["Email", " EMAIL ", "e-mail", "E_Mail", "e.mail", "E - mail"]) assert.equal(normaliseHeader(h), "email", h);
});

test("the alias table: every alias is already normalised and claims one role only", () => {
  const owner = new Map<string, string>();
  for (const [role, list] of Object.entries(ALIASES)) {
    for (const a of list) {
      assert.equal(normaliseHeader(a), a, a);
      assert.ok(!owner.has(a), `${a} in ${role} and ${owner.get(a)}`);
      owner.set(a, role);
    }
  }
});

for (const layout of LAYOUTS) {
  test(`detects: ${layout.label}`, () => {
    const r = readRows(layout.csv, 10);
    assert.ok(!("error" in r), JSON.stringify(r));
    assert.deepEqual(r.detected.columns, layout.columns);
    const names = r.detected.ignored.filter((i) => i.reason === NAMES_NOT_STORED).map((i) => i.header);
    assert.deepEqual(names, layout.names);
    assert.deepEqual(r.detected.ambiguous, layout.ambiguous ?? []);
    assert.equal(r.detected.headerless, false);
    assert.equal(r.rows.length + r.invalid.length, 1);
  });
}

test("detected: delimiter, unrecognised and losing columns are reported", () => {
  const r = readRows("Email;Created;Opt-in time;Score;email\na@example.com;2024-01-01;2024-01-02;3;b@example.com\n", 10);
  assert.ok(!("error" in r));
  assert.equal(r.detected.delimiter, ";");
  assert.equal(r.detected.columns.signedUpAt, "Opt-in time");
  assert.deepEqual(r.detected.ambiguous, [
    { role: "email", chosen: "Email", alsoMatched: ["email"] },
    { role: "signedUpAt", chosen: "Opt-in time", alsoMatched: ["Created"] },
  ]);
  assert.deepEqual(r.detected.ignored, [
    { header: "Created", code: "used_elsewhere", reason: 'ignored: "Opt-in time" was used for signedUpAt' },
    { header: "Score", code: "not_recognised", reason: NOT_RECOGNISED },
    { header: "email", code: "used_elsewhere", reason: 'ignored: "Email" was used for email' },
  ]);
  assert.equal(r.rows[0].email, "a@example.com");
  assert.equal(r.rows[0].signedUpAt, "2024-01-02T00:00:00.000Z");
  // Every header in file order, used or not, for the panel's mapping editor.
  assert.deepEqual(r.detected.headers, ["Email", "Created", "Opt-in time", "Score", "email"]);
});

test("detected: the date order used is reported per date column, the request's when given", () => {
  const inferred = readRows("email,signup date\na@example.com,25/12/2024\n", 10);
  assert.ok(!("error" in inferred));
  assert.deepEqual(inferred.detected.dateOrder, { signedUpAt: "dmy" });
  const given = readRows("email,signup date\na@example.com,01/02/2024\n", 10, { dateOrder: "mdy" });
  assert.ok(!("error" in given));
  assert.deepEqual(given.detected.dateOrder, { signedUpAt: "mdy" });
  assert.equal(given.rows[0].signedUpAt, "2024-01-02T00:00:00.000Z");
  const headerless = readRows("a@example.com,x\n", 10);
  assert.ok(!("error" in headerless));
  assert.deepEqual(headerless.detected.headers, ["column 1", "column 2"]);
});

test("override: a mapping wins over detection; null switches a role off", () => {
  const csv = "Contact,Email,Kind,Groups\nreal@example.com,old@example.com,unsubscribed,bad tag\n";
  const plain = readRows(csv, 10);
  assert.ok(!("error" in plain));
  assert.equal(plain.rows.length, 0); // the detected Email column holds an address, but Groups has a bad tag
  const r = readRows(csv, 10, { mapping: { email: "contact", status: "Kind", tags: null } });
  assert.ok(!("error" in r));
  assert.deepEqual(r.detected.columns, { ...Object.fromEntries(ROLES.map((x) => [x, null])), email: "Contact", status: "Kind" });
  assert.deepEqual(r.rows, [{ line: 2, email: "real@example.com", language: null, tags: "", status: "unsubscribed" }]);
  assert.deepEqual(r.detected.ignored.map((i) => i.header), ["Email", "Groups"]);
});

test("override errors", () => {
  const cols = (m: Record<string, string | null>) => detectColumns(["Email", "Mail"], m, ",", false);
  assert.equal("error" in cols({ email: "Nope" }) && cols({ email: "Nope" }).error, "invalid_mapping");
  const twice = cols({ email: "Email", status: "email" });
  assert.ok("error" in twice && /two roles/.test(twice.detail));
  const off = cols({ email: null });
  assert.ok("error" in off && off.error === "missing_email_column");
  for (const bad of ["x", [1], { name: "Name" }, { email: 5 }]) {
    const m = parseMapping(bad);
    assert.ok("error" in m && m.error === "invalid_mapping", JSON.stringify(bad));
  }
  assert.deepEqual(parseMapping(undefined), {});
  assert.deepEqual(parseMapping(null), {});
  assert.deepEqual(parseMapping({ email: "E", tags: null }), { email: "E", tags: null });
});

test("headerless: line 1 is an address → single-column list, extra columns ignored", () => {
  const r = readRows("first@example.com\nsecond@example.com\n\nnot-an-address\n", 10);
  assert.ok(!("error" in r));
  assert.equal(r.detected.headerless, true);
  assert.equal(r.total, 3);
  assert.deepEqual(r.rows.map((x) => [x.line, x.email]), [[1, "first@example.com"], [2, "second@example.com"]]);
  assert.deepEqual(r.invalid, [{ line: 4, reason: "invalid_email", value: "not-an-address" }]);
  const two = readRows("a@example.com;pl\nb@example.com;en\n", 10);
  assert.ok(!("error" in two));
  assert.equal(two.detected.delimiter, ";");
  assert.deepEqual(two.detected.ignored, [{ header: "column 2", code: "not_recognised", reason: NOT_RECOGNISED }]);
  assert.deepEqual(two.rows.map((x) => x.language), [null, null]);
  // A mapping can still use the made-up names.
  const mapped = readRows("a@example.com;pl\n", 10, { mapping: { language: "column 2" } });
  assert.ok(!("error" in mapped) && mapped.rows[0].language === "pl");
  const moved = readRows("x;a@example.com\n", 10, { mapping: { email: "column 2" } });
  assert.ok("error" in moved); // line 1 has no address first: it is a header row, "column 2" is not in it
  const off = readRows("a@example.com\n", 10, { mapping: { email: null } });
  assert.ok("error" in off && off.error === "missing_email_column");
});
