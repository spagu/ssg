// Signing the shop's order webhooks, as UCP requires of every
// business-to-platform webhook (overview.md: "Business-to-platform webhooks
// MUST be signed"; order.md, "Webhook Signature Verification";
// signatures.md, "REST Request Signing").
//
//   Content-Digest   sha-256=:<base64 SHA-256 of the raw body>:   (RFC 9530)
//   Signature-Input  sig1=("@method" "@authority" "@path" ["@query"] "ucp-agent"
//                    "idempotency-key" "content-digest" "content-type");created=<unix>;keyid="<kid>"
//   Signature        sig1=:<base64 ES256 signature, raw r||s>:      (RFC 9421)
//
// The key is ES256 (ECDSA P-256), the protocol's universal baseline. Its
// private half is the SHOP_UCP_SIGNING_KEY secret (a JWK with `d` and `kid`);
// its public half is published in /.well-known/ucp `signing_keys`, where the
// platform looks it up by `kid`. WebCrypto's ECDSA signature is already the
// fixed-width r||s the specification demands (not DER).

import type { Env } from "./_env";

interface EcJwk {
  kty: "EC";
  crv: "P-256";
  x: string;
  y: string;
  d: string;
  kid: string;
}

/** The configured key, or null when it is missing or not an EC P-256
 *  private JWK with a kid. */
export function signingJwk(env: Env): EcJwk | null {
  if (!env.SHOP_UCP_SIGNING_KEY) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(env.SHOP_UCP_SIGNING_KEY);
  } catch {
    return null;
  }
  const k = parsed as Partial<EcJwk> | null;
  const field = (v: unknown) => typeof v === "string" && v.length > 0;
  if (!k || k.kty !== "EC" || k.crv !== "P-256" || !field(k.x) || !field(k.y) || !field(k.d) || !field(k.kid)) return null;
  return k as EcJwk;
}

/** The public half, as signatures.md's "EC Key Structure". Never `d`. */
export function publicJwk(env: Env): Record<string, string> | null {
  const k = signingJwk(env);
  if (!k) return null;
  return { kid: k.kid, kty: "EC", crv: "P-256", x: k.x, y: k.y, use: "sig", alg: "ES256" };
}

const base64 = (bytes: ArrayBuffer): string => btoa(String.fromCharCode(...new Uint8Array(bytes)));

/** RFC 9421 headers for a POST of `body` (raw bytes as sent) to `url`. */
export async function signedHeaders(
  env: Env,
  url: URL,
  body: string,
  headers: { "ucp-agent": string; "idempotency-key": string; "content-type": string },
): Promise<Record<string, string>> {
  const jwk = signingJwk(env);
  if (!jwk) throw new Error("SHOP_UCP_SIGNING_KEY is missing or not an EC P-256 private JWK with a kid");

  const digest = `sha-256=:${base64(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(body)))}:`;
  const derived: Array<[string, string]> = [
    ["@method", "POST"],
    ["@authority", url.host.toLowerCase()],
    ["@path", url.pathname],
    ...(url.search ? ([["@query", url.search]] as Array<[string, string]>) : []),
    ["ucp-agent", headers["ucp-agent"]],
    ["idempotency-key", headers["idempotency-key"]],
    ["content-digest", digest],
    ["content-type", headers["content-type"]],
  ];
  const params = `(${derived.map(([name]) => `"${name}"`).join(" ")});created=${Math.floor(Date.now() / 1000)};keyid="${jwk.kid}"`;
  const base = [...derived.map(([name, value]) => `"${name}": ${value}`), `"@signature-params": ${params}`].join("\n");

  const key = await crypto.subtle.importKey(
    "jwk",
    { kty: "EC", crv: "P-256", x: jwk.x, y: jwk.y, d: jwk.d, ext: true },
    { name: "ECDSA", namedCurve: "P-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, key, new TextEncoder().encode(base));
  return {
    "content-digest": digest,
    "signature-input": `sig1=${params}`,
    signature: `sig1=:${base64(signature)}:`,
  };
}
