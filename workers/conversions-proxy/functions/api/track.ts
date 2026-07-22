// Cloudflare Pages Function: server-side conversions relay (Meta CAPI).
// POST /api/track  { "event": "Purchase", "email": "…", "value": 49, "currency": "USD" }
// Keeps the access token server-side and hashes PII with SHA-256 before it
// leaves the edge, as the Conversions API requires. No npm dependencies.
//
// Secrets:
//   META_PIXEL_ID       your pixel / dataset id
//   META_ACCESS_TOKEN   CAPI access token
//   (Pinterest variant is commented below — set PINTEREST_* and swap the URL.)

interface Env {
  META_PIXEL_ID: string;
  META_ACCESS_TOKEN: string;
}

const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json" } });

async function sha256Hex(value: string): Promise<string> {
  const norm = value.trim().toLowerCase();
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(norm));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  let payload: { event?: string; email?: string; value?: number; currency?: string };
  try {
    payload = await request.json();
  } catch {
    return json({ error: "invalid JSON body" }, 400);
  }
  if (!payload.event) return json({ error: "event is required" }, 422);

  const userData: Record<string, string[]> = {};
  if (payload.email) userData.em = [await sha256Hex(payload.email)];
  const ip = request.headers.get("cf-connecting-ip");
  const ua = request.headers.get("user-agent");

  const body = {
    data: [
      {
        event_name: payload.event,
        event_time: Math.floor(Date.now() / 1000),
        action_source: "website",
        event_source_url: request.headers.get("referer") ?? undefined,
        user_data: {
          ...userData,
          client_ip_address: ip ?? undefined,
          client_user_agent: ua ?? undefined,
        },
        custom_data:
          payload.value != null ? { value: payload.value, currency: payload.currency ?? "USD" } : undefined,
      },
    ],
  };

  const url = `https://graph.facebook.com/v19.0/${env.META_PIXEL_ID}/events?access_token=${env.META_ACCESS_TOKEN}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });

  // Pinterest CAPI variant:
  // const url = `https://api.pinterest.com/v5/ad_accounts/${env.PINTEREST_AD_ACCOUNT}/events`;
  // headers: { authorization: `Bearer ${env.PINTEREST_ACCESS_TOKEN}`, ... }

  return res.ok ? json({ ok: true }) : json({ error: "relay failed" }, 502);
};
