// The business profile at /.well-known/ucp (overview.md, "Business Profile";
// Google, "Publish your UCP profile", version 2026-04-08, read 2026-11-14).
//
// It tells the platform which protocol version this shop speaks, where the
// REST endpoint is, which capabilities it offers (checkout, and order for
// the webhooks), the Google Pay handler, and the public key that verifies the
// shop's signed order webhooks. It must be public — no X-API-Key — and it is
// built from configuration on every request, so a rotated key or a new
// merchant id shows up without a deploy.

import type { Env } from "./_env";
import { shopOrigin } from "./_fulfil";
import { json } from "./_lib";
import { ensureSchema } from "./_schema";
import { allSettings } from "./_settings";
import { ucpMissing } from "./_ucp-auth";
import { CHECKOUT_CAPABILITY, ORDER_CAPABILITY, UCP_VERSION, ucpEnabled } from "./_ucp-http";
import { googlePayHandler, HANDLER_NAME } from "./_ucp-payment";
import { merchantName } from "./_ucp-render";
import { publicJwk } from "./_ucp-sign";

const SPEC = `https://ucp.dev/${UCP_VERSION}`;

export async function businessProfile(env: Env, origin: string): Promise<Record<string, unknown>> {
  const settings = await allSettings(env);
  return {
    ucp: {
      version: UCP_VERSION,
      services: {
        "dev.ucp.shopping": [
          {
            version: UCP_VERSION,
            spec: `${SPEC}/specification/overview`,
            transport: "rest",
            endpoint: `${origin}/api/shop/ucp`,
            schema: `${SPEC}/services/shopping/rest.openapi.json`,
          },
        ],
      },
      capabilities: {
        [CHECKOUT_CAPABILITY]: [
          { version: UCP_VERSION, spec: `${SPEC}/specification/checkout`, schema: `${SPEC}/schemas/shopping/checkout.json` },
        ],
        [ORDER_CAPABILITY]: [
          { version: UCP_VERSION, spec: `${SPEC}/specification/order`, schema: `${SPEC}/schemas/shopping/order.json` },
        ],
      },
      payment_handlers: { [HANDLER_NAME]: [googlePayHandler(env, origin, merchantName(settings))] },
    },
    signing_keys: [publicJwk(env)],
  };
}

export const onProfileGet: PagesFunction<Env> = async ({ request, env }) => {
  if (!ucpEnabled(env)) return new Response("Not Found", { status: 404 });
  if (!env.SHOP_DB || ucpMissing(env).length > 0) {
    return json({ code: "not_configured", content: "Universal Commerce Protocol checkout is not fully configured." }, 503, { "retry-after": "300" });
  }
  await ensureSchema(env);
  const origin = await shopOrigin(env, request);
  // Cached briefly: platforms "SHOULD cache the profile according to HTTP
  // cache-control directives", and a key rotation should not wait long.
  return json(await businessProfile(env, origin), 200, { "cache-control": "public, max-age=300" });
};
