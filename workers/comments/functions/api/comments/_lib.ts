// Shared helpers for the comments worker. A leading underscore keeps this file
// out of the Pages route table — it is imported, never served.

import { accessIdentity } from "./_access";
import { authAdmin, csrfProblem, granted } from "./_admin-auth";
import type { Admin, Operation } from "./_admin-auth";
import { configFrom } from "./_session-verifier";

export interface Env {
  COMMENTS_DB: D1Database;
  COMMENTS_TURNSTILE_SECRET?: string; // this worker's own Turnstile secret
  TURNSTILE_SECRET?: string; // shared fallback, read only when the prefixed one is unset
  COMMENTS_ADMIN_PASSWORD?: string; // moderation via HTTP Basic (fallback when Access is not set)
  COMMENTS_ACCESS_TEAM?: string; // Cloudflare Access team ("myteam" or "myteam.cloudflareaccess.com")
  COMMENTS_ACCESS_AUD?: string; // Cloudflare Access application AUD tag
  // Sessions of the `auth` worker (GO-136, workers/auth/docs/CONTRACT.md); any one set turns the mode on.
  COMMENTS_AUTH_ISSUER?: string; // the `iss` to accept, e.g. https://example.com/api/auth
  COMMENTS_AUTH_LOGIN_URL?: string; // where a moderator without a session signs in, e.g. /auth/login
  COMMENTS_AUTH_PUBLIC_KEY?: string; // auth's public JWK or JWKS (recommended) — or:
  COMMENTS_AUTH_JWKS_URL?: string; // /api/auth/jwks or a same-origin https:// URL
  COMMENTS_AUTH_AUDIENCE?: string; // the `aud` to accept; default the request's host
  COMMENTS_IP_SALT?: string;
  COMMENTS_ORDER?: string; // "newest" | "oldest"
  COMMENTS_CLOSE_AFTER_DAYS?: string; // auto-close a thread after N days of inactivity (0/unset = never)
  COMMENTS_RETENTION_DAYS?: string; // purge spam/pending older than N days (0/unset = keep forever)
  COMMENTS_AKISMET_KEY?: string;
  COMMENTS_AKISMET_URL?: string;
  // Email-on-new-comment via an HTTP email API (e.g. Resend, or your own relay).
  // A moderation notice is sent when a non-spam comment lands. No-op unless
  // MAIL_URL + MAIL_FROM + MAIL_TO are set (#1.8.16).
  COMMENTS_MAIL_URL?: string; // POST endpoint accepting {from,to,subject,text}
  COMMENTS_MAIL_KEY?: string; // bearer token for the email API
  COMMENTS_MAIL_FROM?: string; // sender address
  COMMENTS_MAIL_TO?: string; // comma-separated recipient(s)
  COMMENTS_MAIL_SUBJECT?: string; // optional subject prefix
}

export interface CommentRow {
  id: string;
  url: string;
  author: string;
  body: string;
  status: string;
  created_at: string;
  avatar_hash: string | null;
  parent_id: string | null; // the comment this replies to, or null for a top-level comment
}

export const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  });

export async function sha256hex(input: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// turnstileSecret picks the secret the submit token is verified with. Every
// worker in one Pages project shares one environment, so an unprefixed secret
// set for one template is read by all of them (#325). The prefixed name is this
// worker's own; the shared TURNSTILE_SECRET stays as a fallback so projects
// configured before the prefix existed keep working.
export function turnstileSecret(env: Env): string | undefined {
  return env.COMMENTS_TURNSTILE_SECRET || env.TURNSTILE_SECRET;
}

export async function verifyTurnstile(secret: string, token: string, ip: string | null): Promise<boolean> {
  const body = new FormData();
  body.append("secret", secret);
  body.append("response", token);
  if (ip) body.append("remoteip", ip);
  const res = await fetch("https://challenges.cloudflare.com/turnstile/v0/siteverify", {
    method: "POST",
    body,
  });
  const out = (await res.json()) as { success: boolean };
  return out.success === true;
}

// Only same-origin page paths are accepted as a comment target, so the table
// cannot be seeded with arbitrary or off-site URLs. Rejecting a leading "//" or
// "/\" (and any backslash) closes the open-redirect where a browser reads
// "/\evil.com" as "//evil.com" → https://evil.com/ when a moderator clicks the
// stored link in the admin panel.
// Control characters and spaces are refused too (security review SR-11): URL
// parsers drop tabs and newlines, so "/\t/evil.example" is "//evil.example".
export function normaliseURL(raw: unknown): string | null {
  if (typeof raw !== "string" || !raw.startsWith("/") || raw.includes("\\") || /[\u0000-\u0020\u007f]/.test(raw)) return null;
  if (raw.startsWith("//")) return null;
  // Never empty: what is left still starts with the "/" checked above.
  return raw.split(/[?#]/)[0].slice(0, 512);
}

// Auto-close window in milliseconds (0 = never), from COMMENTS_CLOSE_AFTER_DAYS.
export function closeWindowMs(env: Env): number {
  const days = Number.parseInt(env.COMMENTS_CLOSE_AFTER_DAYS || "0", 10);
  return Number.isFinite(days) && days > 0 ? days * 86400000 : 0;
}

// isClosed decides whether a thread still accepts comments. It closes once the
// window has elapsed since the thread's last activity: the newest comment when
// the thread has any, else the post's publish date for a thread with none yet.
//
// The newest comment (server-side, from D1) GOVERNS whenever it exists — the
// client-supplied publishedISO is consulted ONLY for an empty thread. Otherwise
// a raw POST carrying a far-future `published` could out-vote a years-old last
// comment and keep a closed thread open (auto-close bypass). For an empty thread
// the publish date is the only anchor available; forging it there merely lets a
// first comment onto an old-but-empty post, which is harmless.
export function isClosed(windowMs: number, lastActivityISO: string | null, publishedISO: string | null): boolean {
  if (windowMs <= 0) return false;
  const anchorISO = lastActivityISO != null ? lastActivityISO : publishedISO;
  if (typeof anchorISO !== "string") return false; // no comments, no known date → open
  const t = Date.parse(anchorISO);
  if (Number.isNaN(t)) return false;
  return Date.now() - t > windowMs;
}

// A cheap heuristic spam score (0..1) for when Akismet is not configured:
// link-stuffing and known keyword patterns are the usual drive-by comment spam.
export function heuristicSpam(author: string, body: string): number {
  const text = `${author}\n${body}`.toLowerCase();
  let score = 0;
  const links = (body.match(/https?:\/\//g) || []).length;
  if (links >= 2) score += 0.5;
  if (links >= 4) score += 0.3;
  if (/\b(viagra|casino|loan|crypto|forex|porn|escort)\b/.test(text)) score += 0.6;
  if (/\[url=|\[link=/.test(text)) score += 0.5;
  if (body.length < 3) score += 0.3;
  return Math.min(score, 1);
}

// isSpam runs Akismet when configured, else the heuristic. A failed Akismet call
// falls back to the heuristic rather than blocking a legitimate comment.
export async function isSpam(env: Env, author: string, email: string, body: string, ip: string | null, ua: string): Promise<boolean> {
  if (env.COMMENTS_AKISMET_KEY && env.COMMENTS_AKISMET_URL) {
    try {
      const form = new URLSearchParams({
        api_key: env.COMMENTS_AKISMET_KEY,
        comment_author: author,
        comment_author_email: email,
        comment_content: body,
        comment_type: "comment",
        user_ip: ip || "",
        user_agent: ua,
        blog: "https://example.com",
      });
      const res = await fetch(env.COMMENTS_AKISMET_URL, {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: form,
      });
      const verdict = (await res.text()).trim();
      if (verdict === "true" || verdict === "false") return verdict === "true";
    } catch {
      /* fall through to the heuristic */
    }
  }
  return heuristicSpam(author, body) >= 0.7;
}

// requireAdmin gates the moderation endpoints. Three modes, the first that is
// configured wins (workers/auth/docs/CONTRACT.md, "Admin modes"):
//   - Cloudflare Access when COMMENTS_ACCESS_TEAM + AUD are set: the panel sits
//     behind your SSO and Access forwards a signed JWT we verify;
//   - auth when any COMMENTS_AUTH_* is set: the `auth` worker's session, with
//     per-operation permissions (_admin-auth.ts). A half-configured set is a
//     503 naming what is missing — never a fall-through to the password, and
//     the password is not accepted while this mode is on (it would let anyone
//     who knows it past every permission, unrecorded);
//   - HTTP Basic with COMMENTS_ADMIN_PASSWORD otherwise.
// Returns who passed (Admin: mode, user, perms, actor) or the Response to send.
// `op` checks that operation's permission in auth mode; Access and the
// password allow every operation, as before.
export async function requireAdmin(request: Request, env: Env, op?: Operation): Promise<Response | Admin> {
  if (env.COMMENTS_ACCESS_TEAM && env.COMMENTS_ACCESS_AUD) {
    const who = await accessIdentity(request, env);
    if (who instanceof Response) return who;
    // Access lets through any request carrying a valid CF_Authorization
    // cookie, which the browser attaches by itself — a cross-site or sibling
    // form included — so a write needs the same CSRF proof as auth mode
    // (security review SR-10).
    if (!["GET", "HEAD"].includes(request.method)) {
      const problem = csrfProblem(request);
      if (problem) return json({ error: problem }, 403);
    }
    return granted("access", who.email);
  }
  const auth = configFrom(env, "COMMENTS");
  if (auth) return authAdmin(request, auth, op);
  const expected = env.COMMENTS_ADMIN_PASSWORD;
  if (!expected) return json({ error: "moderation not configured" }, 503);
  const header = request.headers.get("authorization") || "";
  if (header.startsWith("Basic ")) {
    try {
      const decoded = atob(header.slice(6));
      const pass = decoded.slice(decoded.indexOf(":") + 1);
      if (timingSafeEqual(pass, expected)) return granted("password", null);
    } catch {
      /* malformed header falls through to a 401 */
    }
  }
  // Deliberately NO WWW-Authenticate header: it would make the browser pop its
  // native Basic-auth dialog on top of the panel's own password field. curl -u
  // sends the credential proactively, so it doesn't need the challenge.
  return json({ error: "unauthorized" }, 401);
}

// Constant-time compare that folds a length difference into the accumulator
// instead of returning early, so the length of the configured secret `b` does
// not leak through timing (only `a`'s length, which the caller already chose,
// affects the iteration count). Callers guarantee `b` is non-empty.
function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}

// Per-isolate timestamp of the last purge: the lazy purge rides on new
// comments and runs at most hourly per isolate, so a busy thread does not
// turn into a DELETE per request (#327).
let lastPurge = 0;
const PURGE_EVERY_MS = 60 * 60 * 1000;

// purgeExpired deletes spam and pending comments older than
// COMMENTS_RETENTION_DAYS. Approved comments are the site's content and are
// never purged. `force` skips the hourly throttle (the admin endpoint).
// Returns the rows deleted.
export async function purgeExpired(env: Env, force = false): Promise<number> {
  const days = Number.parseInt(env.COMMENTS_RETENTION_DAYS || "0", 10);
  if (!Number.isFinite(days) || days <= 0) return 0;
  if (!force && Date.now() - lastPurge < PURGE_EVERY_MS) return 0;
  lastPurge = Date.now();
  const cutoff = new Date(Date.now() - days * 86400000).toISOString();
  const res = await env.COMMENTS_DB.prepare(
    "DELETE FROM comments WHERE status IN ('spam', 'pending') AND created_at < ?",
  ).bind(cutoff).run();
  return res.meta.changes ?? 0;
}
