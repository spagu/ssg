// A demonstration consumer (GO-135): the admin gate of a Pages template that uses `auth`, built only
// from its byte-identical copy of the portable verifier (_session-verifier.ts, guarded by
// test/demo-consumer.test.ts). Modes in the contract's order: Cloudflare Access → auth → password.
import { can, configFrom, denied, forbidden, verifySession } from "./_session-verifier";

interface Env { DEMO_ADMIN_PASSWORD?: string; [name: string]: unknown }
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const digest = async (s: string) => new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s)));

// gate answers who may do `permission`, or the Response to send instead.
async function gate(request: Request, env: Env, permission: string): Promise<{ who: string } | Response> {
  // 1. Cloudflare Access, when DEMO_ACCESS_TEAM + AUD are set: the template's own verifyAccess (not shown).
  const auth = configFrom(env, "DEMO"); // 2. auth: null when no DEMO_AUTH_* is set
  if (auth) {
    const v = await verifySession(request, auth);
    if (!v.ok) return denied(request, auth, v.reason);
    return can(v.perms, permission) ? { who: v.user.email } : forbidden(permission);
  }
  if (!env.DEMO_ADMIN_PASSWORD) return json({ error: "not_configured" }, 503); // 3. the shared password
  const [given, want] = await Promise.all([digest(request.headers.get("authorization") ?? ""), digest(`Basic ${btoa(`admin:${env.DEMO_ADMIN_PASSWORD}`)}`)]);
  return given.every((b, i) => b === want[i]) ? { who: "password" } : json({ error: "unauthorized" }, 401);
}

export const onRequestGet = async ({ request, env }: { request: Request; env: Env }): Promise<Response> => {
  const g = await gate(request, env, "demo:read");
  return g instanceof Response ? g : json({ hello: g.who });
};
