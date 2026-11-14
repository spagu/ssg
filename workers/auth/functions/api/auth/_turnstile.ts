// Cloudflare Turnstile, asked for only after repeated failed logins and only
// when a secret is configured. A leading underscore keeps this file out of the
// Pages route table. A network failure counts as "not verified": the check
// exists to slow automated guessing, so it fails closed.

export const SITEVERIFY = "https://challenges.cloudflare.com/turnstile/v0/siteverify";

export async function verifyTurnstile(secret: string, token: string, ip: string): Promise<boolean> {
  if (!token) return false;
  const body = new FormData();
  body.append("secret", secret);
  body.append("response", token);
  body.append("remoteip", ip);
  try {
    const res = await fetch(SITEVERIFY, { method: "POST", body });
    const out = (await res.json()) as { success?: unknown };
    return out.success === true;
  } catch {
    return false;
  }
}
