import { env } from "./_env";
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { ensureSchema } from "../functions/api/shop/_schema";

describe("the runtime the tests run in", () => {
  it("has the bindings the shop needs", async () => {
    await ensureSchema(env);
    const row = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM vat_rates`).first<{ n: number }>();
    assert.ok((row?.n) > 0);
  });
});
