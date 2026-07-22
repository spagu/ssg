// Cloudflare Pages Function: verify and handle Stripe webhooks.
// POST /api/stripe-webhook — validates the Stripe-Signature header with WebCrypto
// HMAC-SHA256 (no SDK), then acts on the event. No npm dependencies.
//
// Secrets:
//   STRIPE_WEBHOOK_SECRET   whsec_... from the Stripe dashboard endpoint

interface Env {
  STRIPE_WEBHOOK_SECRET: string;
}

// Stripe signs `${timestamp}.${rawBody}`; the header carries t= and v1=.
function parseSigHeader(header: string): { t: string; v1: string[] } {
  const parts = header.split(",").map((p) => p.trim().split("="));
  const t = parts.find(([k]) => k === "t")?.[1] ?? "";
  const v1 = parts.filter(([k]) => k === "v1").map(([, v]) => v);
  return { t, v1 };
}

async function hmacHex(secret: string, message: string): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(message));
  return [...new Uint8Array(sig)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= (a.codePointAt(i) ?? 0) ^ (b.codePointAt(i) ?? 0);
  return diff === 0;
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const header = request.headers.get("stripe-signature");
  if (!header) return new Response("missing signature", { status: 400 });
  const raw = await request.text();
  const { t, v1 } = parseSigHeader(header);
  if (!t || v1.length === 0) return new Response("malformed signature", { status: 400 });

  const expected = await hmacHex(env.STRIPE_WEBHOOK_SECRET, `${t}.${raw}`);
  if (!v1.some((sig) => timingSafeEqual(sig, expected))) {
    return new Response("signature mismatch", { status: 400 });
  }

  const event = JSON.parse(raw) as { type: string; data: { object: unknown } };
  const order = event.data.object;
  let action = "ignored";
  if (event.type === "checkout.session.completed") {
    // Fulfil the order here: mark paid, grant access, send a receipt (use `order`).
    action = order ? "order-fulfilled" : "ignored";
  } else if (event.type === "invoice.paid") {
    // Extend the subscription period here (use `order`).
    action = order ? "subscription-extended" : "ignored";
  }
  return new Response(JSON.stringify({ received: true, action }), {
    headers: { "content-type": "application/json" },
  });
};
