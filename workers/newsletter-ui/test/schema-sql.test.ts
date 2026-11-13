// schema.sql and src/schema.ts must describe the same tables: someone who
// applies the file by hand gets what the worker would have created.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ensureSchema } from "../src/schema.ts";
import { fakeD1 } from "./fakes.ts";

const shape = (d1: ReturnType<typeof fakeD1>) => {
  const objects = d1.sqlite.prepare("SELECT type, name, tbl_name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name").all();
  return objects.map((o) => ({
    ...o,
    columns: o.type === "table" ? d1.sqlite.prepare(`PRAGMA table_info(${o.name})`).all().map((c) => ({ ...c })) : [],
  }));
};

test("schema.sql matches the runtime schema", async () => {
  const fromCode = fakeD1();
  await ensureSchema({ NEWSLETTER_DB: fromCode } as never);
  const fromFile = fakeD1();
  fromFile.sqlite.exec(readFileSync(new URL("../schema.sql", import.meta.url), "utf8"));
  assert.deepEqual(shape(fromFile), shape(fromCode));
  const statements = readFileSync(new URL("../schema.sql", import.meta.url), "utf8").replace(/--.*$/gm, "");
  assert.doesNotMatch(statements, /\bsubscribers\b/, "the file never touches the newsletter template's table");
});
