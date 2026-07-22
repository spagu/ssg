// Cloudflare Pages Function: dynamic price lookup.
// GET /api/price/:sku — returns the current price for a SKU. Reads a KV
// namespace when bound (fast, edge-cached), otherwise falls back to an upstream
// pricing API. JSON out; consumed by /js/price.js on the static page.
//
// Bindings / secrets:
//   PRICES        (optional) KV namespace: key = sku, value = JSON {amount,currency}
//   PRICE_API_URL (optional) upstream endpoint, called as `${PRICE_API_URL}/${sku}`
//   PRICE_API_KEY (optional) bearer token for the upstream API

interface Env {
  PRICES?: KVNamespace;
  PRICE_API_URL?: string;
  PRICE_API_KEY?: string;
}

const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json", "cache-control": "public, max-age=60" },
  });

export const onRequestGet: PagesFunction<Env> = async ({ params, env }) => {
  const sku = String(params.sku ?? "").trim();
  if (!sku || !/^[A-Za-z0-9._-]+$/.test(sku)) return json({ error: "invalid sku" }, 400);

  if (env.PRICES) {
    const cached = await env.PRICES.get(sku);
    if (cached) return json({ sku, ...JSON.parse(cached) });
  }

  if (env.PRICE_API_URL) {
    const headers: Record<string, string> = {};
    if (env.PRICE_API_KEY) headers.authorization = `Bearer ${env.PRICE_API_KEY}`;
    const res = await fetch(`${env.PRICE_API_URL}/${encodeURIComponent(sku)}`, { headers });
    if (res.ok) {
      const price = await res.json();
      return json({ sku, ...(price as object) });
    }
  }

  return json({ error: "price not found" }, 404);
};
