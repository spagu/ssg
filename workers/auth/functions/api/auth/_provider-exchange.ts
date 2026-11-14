// The authorization-code exchange every provider shares (RFC 6749 4.1.3,
// RFC 7636 4.5): one POST to the provider's token endpoint with the code, the
// exact redirect URI the authorization request used, the client credentials
// and — with PKCE — the verifier. A leading underscore keeps this file out of
// the Pages route table. The answer (tokens) stays in memory for the callback
// that asked and is never stored or logged.

import type { Exchange } from "./_provider";
import { okJson, providerFetch } from "./_provider-fetch";

export async function exchangeCode(
  tokenUrl: string,
  hosts: readonly string[],
  clientId: string,
  clientSecret: string,
  x: Exchange,
  headers: Record<string, string> = {},
): Promise<{ ok: true; data: Record<string, unknown> } | { ok: false; detail: string }> {
  const form: Record<string, string> = { grant_type: "authorization_code", code: x.code, redirect_uri: x.redirectUri, client_id: clientId, client_secret: clientSecret };
  if (x.verifier) form.code_verifier = x.verifier;
  const got = okJson(await providerFetch(tokenUrl, hosts, { method: "POST", form, headers }), "token endpoint");
  // GitHub answers a refused code with 200 and {error}; nobody else sends an
  // `error` next to a success.
  if (got.ok && typeof got.data.error === "string") return { ok: false, detail: `token endpoint: error ${got.data.error.slice(0, 64)}` };
  return got;
}

// bearer: the Authorization header for an API call with an access token.
export const bearer = (data: Record<string, unknown>): string | null => (typeof data.access_token === "string" && data.access_token ? `Bearer ${data.access_token}` : null);
