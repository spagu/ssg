// The pieces behind the attention list: the same-host join of a module's
// link (a table of attempts to leave the base or the host), the merge (order,
// cap, count) and how one summary answer becomes one module's state.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ATTENTION_MAX, mergeAttention } from "../functions/api/core/_attention.ts";
import { summaryView } from "../functions/api/core/_summary.ts";
import { joinHref } from "../functions/api/core/_target.ts";
import { MANIFESTS } from "./demo.ts";
import { manifest, text } from "./harness.ts";

test("joinHref: only paths under the module's base, as a browser would resolve them", () => {
  const table: [string, string, string | null][] = [
    ["/", "/comments-admin.html", "/comments-admin.html"],
    ["/", "/", "/"],
    ["/newsletter-admin", "/#/campaigns?status=paused", "/newsletter-admin/#/campaigns?status=paused"],
    ["/newsletter-admin/", "/#/", "/newsletter-admin/#/"],
    ["/kitchen", "/a/b/", "/kitchen/a/b/"],
    ["/kitchen", "//evil.example.com/", null],
    ["/kitchen", "https://evil.example.com/", null],
    ["/kitchen", "/../admin", null],
    ["/kitchen", "/a/../../x", null],
    ["/kitchen", "/%2e%2e/x", null],
    ["/kitchen", "/a\\b", null],
    ["/kitchen", "/a?x=1", null],
    ["/kitchen", "/a#<script>", null],
    ["/kitchen", "javascript:alert(1)", null],
    ["/kitchen", "", null],
    ["/kitchen", "/" + "a".repeat(300), null],
    ["https://evil.example.com", "/x", null],
    ["//evil.example.com", "/x", null],
    ["/a/../b", "/x", null],
    ["/kitchen", "/./a", null],
    ["/kitchen", "/a/./b", null],
    ["/kitchen", "/..", null],
    ["/kitchen", "/a/..", null],
    ["/kitchen", "/...", "/kitchen/..."],
    ["/", "/a//b", "/a//b"],
    ["/kitchen", "/#/a/../b", "/kitchen/#/a/../b"],
  ];
  for (const [base, href, want] of table) assert.equal(joinHref(base, href), want, `${base} + ${href}`);
});

test("mergeAttention: capped, counted, stable within a module; a refused link is kept as null", () => {
  const item = (severity: string, n: number, href = "/") => ({ severity, text: text(`${severity} ${n}`), href });
  const s = (id: string, base: string, attention: unknown[]) => ({ id, title: id, base, summary: { contract: 1, generatedAt: "", metrics: [], problems: [], attention } as never });
  const many = Array.from({ length: 10 }, (_, i) => item("info", i));
  const got = mergeAttention([s("b", "/", many), s("a", "/", many), s("c", "//evil", [item("warning", 0)])], "en");
  assert.equal(got.total, 21);
  assert.equal(got.items.length, ATTENTION_MAX);
  assert.deepEqual([got.items[0].module, got.items[0].href], ["c", null]);
  assert.deepEqual(got.items.slice(1, 11).map((a) => a.text), many.map((m) => m.text.en), "module a before b, a's own order kept");
  assert.equal(mergeAttention([s("x", "/", []), { id: "y", title: "y", base: "/", summary: null }], "en").total, 0);
});

test("summaryView: an unknown status is unavailable; an invalid answer without detail has none", () => {
  assert.deepEqual(summaryView({ status: "unknown", ms: null, reason: "no_endpoint" }, "en").view, { state: "unavailable", reason: "no_endpoint" });
  assert.deepEqual(summaryView({ status: "invalid", ms: 1, reason: "not_json", httpStatus: 200 }, "en").view, { state: "invalid", reason: "not_json", httpStatus: 200 });
  assert.ok(MANIFESTS.comments.endpoints.summary);
  assert.equal(manifest("x").id, "x");
});
