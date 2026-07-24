// Shared helpers for the comments worker. A leading underscore keeps this file
// out of the Pages route table — it is imported, never served.

export interface Env {
  COMMENTS_DB: D1Database;
  TURNSTILE_SECRET?: string;
  COMMENTS_ADMIN_PASSWORD?: string;
  COMMENTS_IP_SALT?: string;
  COMMENTS_ORDER?: string; // "newest" | "oldest"
  COMMENTS_CLOSE_AFTER_DAYS?: string; // auto-close a thread after N days of inactivity (0/unset = never)
  COMMENTS_AKISMET_KEY?: string;
  COMMENTS_AKISMET_URL?: string;
}

export interface CommentRow {
  id: string;
  url: string;
  author: string;
  body: string;
  status: string;
  created_at: string;
  avatar_hash: string | null;
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
// cannot be seeded with arbitrary or off-site URLs.
export function normaliseURL(raw: unknown): string | null {
  if (typeof raw !== "string" || !raw.startsWith("/") || raw.startsWith("//")) return null;
  const clean = raw.split(/[?#]/)[0].slice(0, 512);
  return clean || null;
}

// Auto-close window in milliseconds (0 = never), from COMMENTS_CLOSE_AFTER_DAYS.
export function closeWindowMs(env: Env): number {
  const days = parseInt(env.COMMENTS_CLOSE_AFTER_DAYS || "0", 10);
  return Number.isFinite(days) && days > 0 ? days * 86400000 : 0;
}

// isClosed decides whether a thread still accepts comments. It closes once the
// window has elapsed since the thread's last activity, where "activity" is the
// newest comment or — for a thread with none yet — the post's publish date.
// Taking the newest of the two means a recent comment (or a recent post) keeps
// the thread open, and a post with no comments and no known date stays open so
// the first comment is always possible.
export function isClosed(windowMs: number, lastActivityISO: string | null, publishedISO: string | null): boolean {
  if (windowMs <= 0) return false;
  const anchors: number[] = [];
  for (const iso of [lastActivityISO, publishedISO]) {
    if (typeof iso === "string") {
      const t = Date.parse(iso);
      if (!Number.isNaN(t)) anchors.push(t);
    }
  }
  if (!anchors.length) return false;
  return Date.now() - Math.max(...anchors) > windowMs;
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

// requireAdmin gate: HTTP Basic with the configured password (any username).
// Returns null when authorised, or a 401 challenge to return otherwise.
export function requireAdmin(request: Request, env: Env): Response | null {
  const expected = env.COMMENTS_ADMIN_PASSWORD;
  if (!expected) return json({ error: "moderation not configured" }, 503);
  const header = request.headers.get("authorization") || "";
  if (header.startsWith("Basic ")) {
    try {
      const decoded = atob(header.slice(6));
      const pass = decoded.slice(decoded.indexOf(":") + 1);
      if (timingSafeEqual(pass, expected)) return null;
    } catch {
      /* malformed header falls through to a challenge */
    }
  }
  return new Response("Authentication required", {
    status: 401,
    headers: { "www-authenticate": 'Basic realm="comments moderation"' },
  });
}

// Constant-time string compare, so the password check does not leak length or
// content through timing.
function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}
