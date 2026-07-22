// Cloudflare Pages Function: contact / job-application form handler.
// POST /api/contact — validates a Turnstile token, then sends the message via
// MailChannels (no API key required from Cloudflare Pages). Swap in Resend by
// setting RESEND_API_KEY and using the commented block below.
//
// Secrets (wrangler pages secret put <NAME>):
//   TURNSTILE_SECRET   Cloudflare Turnstile secret key
//   CONTACT_TO         destination inbox, e.g. "team@example.com"
//   CONTACT_FROM       verified sender, e.g. "noreply@example.com"
//   RESEND_API_KEY     (optional) enables the Resend path instead of MailChannels

interface Env {
  TURNSTILE_SECRET: string;
  CONTACT_TO: string;
  CONTACT_FROM: string;
  RESEND_API_KEY?: string;
}

const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json" } });

async function verifyTurnstile(secret: string, token: string, ip: string | null): Promise<boolean> {
  const body = new FormData();
  body.append("secret", secret);
  body.append("response", token);
  if (ip) body.append("remoteip", ip);
  const res = await fetch("https://challenges.cloudflare.com/turnstile/v0/siteverify", { method: "POST", body });
  const out = (await res.json()) as { success: boolean };
  return out.success === true;
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  let form: Record<string, string>;
  try {
    const data = await request.formData();
    form = Object.fromEntries([...data.entries()].map(([k, v]) => [k, String(v)]));
  } catch {
    return json({ error: "invalid form data" }, 400);
  }

  const { name, email, message } = form;
  if (!name || !email || !message) return json({ error: "name, email and message are required" }, 422);

  const token = form["cf-turnstile-response"];
  const ip = request.headers.get("cf-connecting-ip");
  if (!token || !(await verifyTurnstile(env.TURNSTILE_SECRET, token, ip))) {
    return json({ error: "captcha verification failed" }, 403);
  }

  const subject = `Contact form: ${name}`;
  const text = `From: ${name} <${email}>\n\n${message}`;

  // Resend path (opt-in):
  // if (env.RESEND_API_KEY) {
  //   const r = await fetch("https://api.resend.com/emails", {
  //     method: "POST",
  //     headers: { authorization: `Bearer ${env.RESEND_API_KEY}`, "content-type": "application/json" },
  //     body: JSON.stringify({ from: env.CONTACT_FROM, to: env.CONTACT_TO, subject, text, reply_to: email }),
  //   });
  //   return r.ok ? json({ ok: true }) : json({ error: "send failed" }, 502);
  // }

  const res = await fetch("https://api.mailchannels.net/tx/v1/send", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      personalizations: [{ to: [{ email: env.CONTACT_TO }] }],
      from: { email: env.CONTACT_FROM, name: "Website" },
      reply_to: { email, name },
      subject,
      content: [{ type: "text/plain", value: text }],
    }),
  });
  return res.ok ? json({ ok: true }) : json({ error: "send failed" }, 502);
};
