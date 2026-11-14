// Reading a provider's variables (GO-151). A leading underscore keeps this
// file out of the Pages route table. Its own module so that the provider
// files (providers/*) need nothing at runtime from _provider.ts, which
// imports them all.

import type { Env } from "./_lib";
import type { Setup } from "./_provider";

// envVar reads AUTH_<ID>_<NAME>, trimmed; "" when unset.
export function envVar(env: Env, id: string, name: string): string {
  const v = (env as unknown as Record<string, unknown>)[`AUTH_${id.toUpperCase()}_${name}`];
  return typeof v === "string" ? v.trim() : "";
}

// clientSetup is the usual case: a client id and a client secret, both
// required; one without the other is "not configured" (null).
export function clientSetup(env: Env, id: string, extra: Record<string, string> = {}): Setup | null {
  const clientId = envVar(env, id, "CLIENT_ID");
  const clientSecret = envVar(env, id, "CLIENT_SECRET");
  return clientId && clientSecret ? { clientId, clientSecret, extra } : null;
}
