// GO-179: secrets as references only (public/js/models/secret-ref.js), in the
// strict forms the worker accepts (src/redact.ts strictReference) — the two
// are checked against each other, so the panel never builds what the worker
// would refuse.
import { test } from "node:test";
import assert from "node:assert/strict";
import { NAME, buildReference, isRedacted, parseReference, secretDraft } from "../public/js/models/secret-ref.js";
import { strictReference } from "../src/redact.ts";

test("isRedacted: exactly the worker's {set, reference}", () => {
  assert.equal(isRedacted({ set: true, reference: "$A" }), true);
  assert.equal(isRedacted({ set: false, reference: null }), true);
  for (const v of [null, "x", [], { set: "yes", reference: null }, { set: true, reference: 3 }, { set: true, reference: null, extra: 1 }, { set: true }]) assert.equal(isRedacted(v), false, JSON.stringify(v));
});

test("parseReference reads each format, and nothing else", () => {
  assert.deepEqual(parseReference("whole", "$JWT"), { login: "", scheme: "", name: "JWT" });
  assert.deepEqual(parseReference("whole", "${JWT}"), { login: "", scheme: "", name: "JWT" });
  assert.deepEqual(parseReference("login", "ada:$PASS"), { login: "ada", scheme: "", name: "PASS" });
  assert.deepEqual(parseReference("inline", "Bearer $T"), { login: "", scheme: "Bearer", name: "T" });
  assert.deepEqual(parseReference("inline", "$T"), { login: "", scheme: "", name: "T" });
  for (const [f, s] of [["whole", "hunter2"], ["whole", "$1X"], ["login", ":$P"], ["login", "ada:pass"], ["login", "$P"], ["inline", "Bearer token here"], ["inline", "Bearer x"], ["whole", 5]]) {
    assert.equal(parseReference(f, s), null, `${f} ${s}`);
  }
});

test("buildReference: the parts become a reference the worker accepts", () => {
  const cases: [string, Record<string, string>, string][] = [
    ["whole", { name: "JWT_SECRET" }, "$JWT_SECRET"],
    ["whole", { name: " $JWT " }, "$JWT"],
    ["login", { name: "PASS", login: "ada" }, "ada:$PASS"],
    ["inline", { name: "TOKEN", scheme: "Bearer" }, "Bearer $TOKEN"],
    ["inline", { name: "TOKEN" }, "$TOKEN"],
  ];
  for (const [format, parts, want] of cases) {
    const r = buildReference(format, parts);
    assert.equal(r.value, want);
    assert.ok(strictReference(format as never, r.value!), `the worker accepts ${want}`);
  }
  assert.deepEqual(buildReference("whole", { name: "my password" }), { error: "secretName" });
  assert.deepEqual(buildReference("whole", {}), { error: "secretName" });
  assert.deepEqual(buildReference("login", { name: "P", login: "a:b" }), { error: "secretLogin" });
  assert.deepEqual(buildReference("login", { name: "P" }), { error: "secretLogin" });
  assert.deepEqual(buildReference("inline", { name: "T", scheme: "Two words" }), { error: "secretScheme" });
  assert.ok(NAME.test("A_1") && !NAME.test("1A"));
});

test("secretDraft: set or not, and whether it can be shown", () => {
  assert.deepEqual(secretDraft("whole", { set: true, reference: "$A" }), { kind: "secret", format: "whole", login: "", scheme: "", name: "A", set: true, keep: false });
  assert.deepEqual(secretDraft("whole", { set: true, reference: null }), { kind: "secret", format: "whole", login: "", scheme: "", name: "", set: true, keep: true });
  assert.deepEqual(secretDraft("whole", undefined), { kind: "secret", format: "whole", login: "", scheme: "", name: "", set: false, keep: false });
  assert.equal(secretDraft("login", "ada:$P").name, "P", "a plain reference string (a revision, a new row)");
  assert.equal(secretDraft("whole", "").set, false);
  assert.equal(secretDraft("whole", "literal").keep, true);
});
