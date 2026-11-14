// Edges of the module-management data (GO-149): the registry's `available`
// list read defensively, change-log rows edited by hand, a position change in
// an import preview.
import { test } from "node:test";
import assert from "node:assert/strict";
import { checkRegistry } from "../functions/api/core/_registry.ts";
import { recentChanges } from "../functions/api/core/_changes.ts";
import { ensureSchema } from "../functions/api/core/_schema.ts";
import { defaults } from "../functions/api/core/_settings.ts";
import { diff } from "../functions/api/core/_transfer.ts";
import { fakeD1 } from "./d1.ts";

test("available: entries in the build's shape are kept, anything else dropped; a missing list is empty", () => {
  const read = (available: unknown) => (checkRegistry({ format: 1, modules: [], available, theme: null }) as { available: unknown }).available;
  assert.deepEqual(read(undefined), []);
  assert.deepEqual(read({ id: "x" }), []);
  assert.deepEqual(
    read([
      { id: "contact-form", kind: "pages-function", title: " Contact form " },
      { id: "newsletter-ui", kind: "worker", title: "" },
      { id: "odd", kind: "plugin", title: "x".repeat(121) },
      { id: "Bad id", kind: "worker", title: "Bad" },
      { id: "a".repeat(65), kind: "worker", title: "Long" },
      "web-push",
      null,
    ]),
    [
      { id: "contact-form", kind: "pages-function", title: "Contact form" },
      { id: "newsletter-ui", kind: "worker", title: "newsletter-ui" },
      { id: "odd", kind: null, title: "odd" },
    ],
  );
});

test("a change-log row edited by hand shows with an empty detail, never breaks the view", async () => {
  const db = fakeD1();
  await ensureSchema(db as never);
  db.sqlite.exec("INSERT INTO core_changes (id, at, actor, module_id, action, detail) VALUES ('1', '2026-11-13T10:00:00Z', 'password', 'a', 'note', '{'), ('2', '2026-11-13T10:01:00Z', 'password', NULL, 'order', '[1]')");
  assert.deepEqual(
    (await recentChanges(db as never)).map((c) => c.detail),
    [{}, {}],
  );
});

test("an import preview lists a position change", () => {
  const settings = new Map([["a", { ...defaults("a"), position: 2 }]]);
  assert.deepEqual(diff(["a"], settings, [{ id: "a", hidden: false, position: 0, labels: {}, note: "" }]), [{ module: "a", field: "position", from: 2, to: 0 }]);
});
