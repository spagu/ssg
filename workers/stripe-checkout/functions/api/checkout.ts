// Cloudflare Pages Function: create a Stripe Checkout Session.
// POST /api/checkout  { "priceId": "price_...", "quantity": 1, "mode": "payment" }
// Talks to the Stripe REST API directly (raw fetch, no SDK) so it needs no npm
// dependencies and deploys via Direct Upload.
//
// Secrets:
//   STRIPE_SECRET_KEY   sk_live_... / sk_test_...
//   CHECKOUT_SUCCESS_URL  e.g. "https://example.com/thank-you"
//   CHECKOUT_CANCEL_URL   e.g. "https://example.com/pricing"

interface Env {
  STRIPE_SECRET_KEY: string;
  CHECKOUT_SUCCESS_URL: string;
  CHECKOUT_CANCEL_URL: string;
}

const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json" } });

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  let payload: { priceId?: string; quantity?: number; mode?: string };
  try {
    payload = await request.json();
  } catch {
    return json({ error: "invalid JSON body" }, 400);
  }
  const priceId = payload.priceId;
  if (!priceId) return json({ error: "priceId is required" }, 422);
  const quantity = Math.max(1, Math.min(payload.quantity ?? 1, 999));
  const mode = payload.mode === "subscription" ? "subscription" : "payment";

  // Stripe expects application/x-www-form-urlencoded with bracketed arrays.
  const form = new URLSearchParams();
  form.set("mode", mode);
  form.set("success_url", `${env.CHECKOUT_SUCCESS_URL}?session_id={CHECKOUT_SESSION_ID}`);
  form.set("cancel_url", env.CHECKOUT_CANCEL_URL);
  form.set("line_items[0][price]", priceId);
  form.set("line_items[0][quantity]", String(quantity));

  const res = await fetch("https://api.stripe.com/v1/checkout/sessions", {
    method: "POST",
    headers: {
      authorization: `Bearer ${env.STRIPE_SECRET_KEY}`,
      "content-type": "application/x-www-form-urlencoded",
    },
    body: form,
  });
  const session = (await res.json()) as { id?: string; url?: string; error?: { message: string } };
  if (!res.ok) return json({ error: session.error?.message ?? "stripe error" }, 502);
  return json({ id: session.id, url: session.url });
};
