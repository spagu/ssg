// Cell values: status vocabularies, every date format, the column-wide
// day/month decision, sign-up source paths.
import { test } from "node:test";
import assert from "node:assert/strict";
import { inferDateOrder, isDateOrder, isSet, normaliseStatus, parseDate, sourcePath } from "../src/import-values.ts";

test("status vocabularies", () => {
  const cases: Record<string, string[]> = {
    active: ["subscribed", "Active", "CONFIRMED", "enabled", "opted_in", "Opt-In", "double opt-in", "verified", "subscriber"],
    pending: ["pending", "Unconfirmed", "not confirmed", "awaiting_confirmation", "unverified"],
    unsubscribed: ["unsubscribed", "Opted Out", "opt-out", "optout", "removed", "inactive", "disabled", "deleted", "archived"],
    bounced: ["bounced", "Cleaned", "invalid", "hard bounce", "Hard-Bounced", "hardbounce", "undeliverable"],
    complaint: ["complained", "complaint", "spam", "Spam complaint", "marked as spam", "ABUSE"],
  };
  for (const [want, words] of Object.entries(cases)) for (const w of words) assert.equal(normaliseStatus(w), want, w);
  for (const w of ["maybe", "soft bounce", "1", ""]) assert.equal(normaliseStatus(w), null, w);
});

test("isSet: placeholders in date columns do not count", () => {
  for (const v of ["", "0", "false", "No", "null", "none", "never", "N/A", "-"]) assert.equal(isSet(v), false, v);
  for (const v of ["2024-01-01", "yes", "x"]) assert.equal(isSet(v), true, v);
});

const NOW = Date.parse("2026-06-01T00:00:00Z");
const iso = (raw: string, order: "dmy" | "mdy" = "dmy") => {
  const d = parseDate(raw, order, NOW);
  return "iso" in d ? d.iso : d.warning;
};

test("dates: every accepted format", () => {
  const cases: [string, string, ("dmy" | "mdy")?][] = [
    ["2024-03-04", "2024-03-04T00:00:00.000Z"],
    ["2024-03-04T05:06:07Z", "2024-03-04T05:06:07.000Z"],
    ["2024-03-04T05:06:07.123+02:00", "2024-03-04T03:06:07.000Z"],
    ["2024-03-04T05:06:07-0130", "2024-03-04T06:36:07.000Z"],
    ["2024-03-04 05:06", "2024-03-04T05:06:00.000Z"],
    ["2024-03-04 05:06:07", "2024-03-04T05:06:07.000Z"],
    ["2024-03-04 05:06:07 UTC", "2024-03-04T05:06:07.000Z"],
    ["2024/03/04", "2024-03-04T00:00:00.000Z"],
    ["2024/3/4 5:06", "2024-03-04T05:06:00.000Z"],
    ["04.03.2024", "2024-03-04T00:00:00.000Z"],
    ["4.3.2024 17:30", "2024-03-04T17:30:00.000Z"],
    ["1709528767", "2024-03-04T05:06:07.000Z"],
    ["1709528767000", "2024-03-04T05:06:07.000Z"],
    ["03/04/2024", "2024-04-03T00:00:00.000Z", "dmy"],
    ["03/04/2024", "2024-03-04T00:00:00.000Z", "mdy"],
    ["03/04/2024 10:00", "2024-03-04T10:00:00.000Z", "mdy"],
  ];
  for (const [raw, want, order] of cases) assert.equal(iso(raw, order), want, raw);
});

test("dates: unreadable, impossible and future values are refused", () => {
  for (const raw of ["yesterday", "2024-13-01", "31.02.2024", "2024-02-30", "2024-01-01 24:00", "2024-01-01 10:60", "2024-01-01 10:00:61", "00.01.2024", "12345678901", "32/01/2024"]) {
    assert.equal(iso(raw), "unparseable", raw);
  }
  assert.equal(iso("13/01/2024", "mdy"), "unparseable");
  assert.equal(iso("2026-06-02"), "future");
  assert.equal(iso("9999999999"), "future");
  assert.equal(iso("2026-06-01T00:00:00Z"), "2026-06-01T00:00:00.000Z"); // now itself is fine
});

test("inferDateOrder: one value decides; never deciding is null, not a guess", () => {
  assert.equal(inferDateOrder(["03/04/2024", "25/04/2024"]), "dmy");
  assert.equal(inferDateOrder(["03/04/2024", "04/25/2024"]), "mdy");
  assert.equal(inferDateOrder(["03/04/2024", "04/05/2024"]), null);
  assert.equal(inferDateOrder(["25/04/2024", "04/25/2024"]), null); // contradictory
  assert.equal(inferDateOrder(["05/05/2024", "2024-01-01", "", "x"]), "dmy"); // reads the same either way
  assert.equal(inferDateOrder([]), "dmy");
  assert.ok(isDateOrder("dmy") && isDateOrder("mdy") && !isDateOrder("ymd") && !isDateOrder(1));
});

test("sourcePath keeps a page path, takes the path of a URL, drops free text", () => {
  assert.equal(sourcePath("/blog/post/?utm=x#top"), "/blog/post/");
  assert.equal(sourcePath("https://example.com/signup/?ref=a"), "/signup/");
  assert.equal(sourcePath("HTTP://example.com"), "/");
  for (const raw of ["", "Landing form", "//evil.example.com/", "/a\\b", "https://", "ftp://example.com/x"]) assert.equal(sourcePath(raw), null, raw);
  assert.equal(sourcePath("/" + "a".repeat(600))?.length, 512);
});
