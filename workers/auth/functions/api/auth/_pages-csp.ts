// The pages' Content-Security-Policy, widened only where a page truly needs
// it (GO-151). A leading underscore keeps this file out of the Pages route
// table. With nothing configured every page sends exactly PAGE_CSP (_pages.ts).
//
//   form-action  a page with login-provider buttons adds the providers'
//                authorization origins: the button is a form POSTed to this
//                worker, which answers 303 to the provider, and browsers apply
//                form-action to the redirect target too — without them the
//                button would be blocked. Only the enabled providers, only on
//                the pages that show their buttons.
//   Turnstile    when — and only when — AUTH_TURNSTILE_SITE_KEY and the
//                Turnstile secret are both set, the login page loads
//                Cloudflare's widget: its script (script-src) and its
//                challenge frame (frame-src), both from
//                https://challenges.cloudflare.com, nothing else (checked in
//                a browser: the widget needs no connect-src, style-src or
//                img-src of its own — its requests run inside its frame).
//                Login page only: it is the one form whose API asks for a
//                Turnstile token.

import type { Env } from "./_lib";
import { PAGE_CSP } from "./_pages";

export const TURNSTILE_ORIGIN = "https://challenges.cloudflare.com";
export const TURNSTILE_SCRIPT = `${TURNSTILE_ORIGIN}/turnstile/v0/api.js`;

// turnstileSiteKey: the site key, when the widget can be both shown and
// checked; null otherwise.
export function turnstileSiteKey(env: Env): string | null {
  const key = (env.AUTH_TURNSTILE_SITE_KEY ?? "").trim();
  return key && (env.AUTH_TURNSTILE_SECRET || env.TURNSTILE_SECRET) ? key : null;
}

export interface CspExtra {
  formAction?: string[];
  turnstile?: boolean;
}

// pageCsp returns PAGE_CSP with the additions; the same string without any.
export function pageCsp(extra: CspExtra = {}): string {
  const add: Record<string, string[]> = { "form-action": extra.formAction ?? [], "script-src": [], "frame-src": [] };
  if (extra.turnstile) {
    add["script-src"].push(TURNSTILE_ORIGIN);
    add["frame-src"].push(TURNSTILE_ORIGIN);
  }
  const directives = PAGE_CSP.split("; ").map((d) => {
    const name = d.split(" ")[0];
    const more = add[name] ?? [];
    delete add[name];
    return more.length ? `${d} ${[...new Set(more)].join(" ")}` : d;
  });
  for (const [name, sources] of Object.entries(add)) if (sources.length) directives.push(`${name} ${sources.join(" ")}`);
  return directives.join("; ");
}
