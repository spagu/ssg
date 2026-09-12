import { env } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import { ensureSchema } from "../functions/api/shop/_schema";

describe("the runtime the tests run in", () => {
  it("has the bindings the shop needs", async () => {
    await ensureSchema(env);
    const row = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM vat_rates`).first<{ n: number }>();
    expect(row?.n).toBeGreaterThan(0);
  });
});
