// Cloudflare Pages Function: comments API.
//   GET  /api/comments?url=/blog/post/   → approved comments for that page
//   POST /api/comments                   → submit a comment (Turnstile-verified,
//                                           spam-checked, stored as `pending`)
//
// No accounts: a comment is a name + optional email + body. Identity is kept for
// compliance ("who and what") as a SALTED hash of the IP plus the user-agent —
// the raw IP (PII) is never stored. New comments are held for moderation.

import {
  Env, CommentRow, json, sha256hex, verifyTurnstile, normaliseURL, isSpam,
  closeWindowMs, isClosed,
} from "./_lib";

// lastActivity is the newest comment on a thread (approved or pending — a
// held comment is still activity). Null when the thread is empty. Only queried
// when auto-close is on, so the common path keeps its single SELECT.
async function lastActivity(env: Env, url: string): Promise<string | null> {
  const row = await env.COMMENTS_DB.prepare(
    `SELECT MAX(created_at) AS last FROM comments
       WHERE url = ? AND status IN ('approved', 'pending')`,
  ).bind(url).first<{ last: string | null }>();
  return row?.last ?? null;
}

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  // Fail clean (JSON 503) rather than throwing an unhandled exception (a raw
  // Cloudflare 500) when the D1 binding is missing — e.g. before it's wired.
  if (!env.COMMENTS_DB) return json({ error: "comments not configured" }, 503);

  const params = new URL(request.url).searchParams;
  const url = normaliseURL(params.get("url"));
  if (!url) return json({ error: "a valid ?url= is required" }, 400);

  const order = env.COMMENTS_ORDER === "oldest" ? "ASC" : "DESC";
  const { results } = await env.COMMENTS_DB.prepare(
    `SELECT id, author, body, created_at, avatar_hash
       FROM comments WHERE url = ? AND status = 'approved'
       ORDER BY created_at ${order} LIMIT 500`,
  ).bind(url).all<CommentRow>();

  // Report whether the thread is closed so the widget can hide the form. The
  // publish date (server-rendered into the widget) anchors an empty thread.
  const windowMs = closeWindowMs(env);
  const closed = windowMs > 0 && isClosed(windowMs, await lastActivity(env, url), params.get("published"));

  return json({ url, count: results.length, comments: results, closed });
};

interface Body {
  url?: string;
  author?: string;
  email?: string;
  body?: string;
  token?: string; // Turnstile response
  published?: string; // post publish date, anchors auto-close for an empty thread
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  let payload: Body;
  try {
    payload = (await request.json()) as Body;
  } catch {
    return json({ error: "invalid JSON" }, 400);
  }

  // Both bindings are required to accept a comment; fail clean before touching
  // D1 (which the close-check below queries) so a missing binding is a 503, not
  // a raw 500.
  if (!env.COMMENTS_DB || !env.TURNSTILE_SECRET) {
    return json({ error: "comments not configured" }, 503);
  }

  const url = normaliseURL(payload.url);
  const author = (payload.author || "").trim().slice(0, 80);
  const email = (payload.email || "").trim().slice(0, 254);
  const body = (payload.body || "").trim().slice(0, 5000);
  if (!url || !author || !email || !body) {
    return json({ error: "url, author, email and body are required" }, 422);
  }
  if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) {
    return json({ error: "a valid email is required" }, 422);
  }

  // Refuse a closed thread before spending a Turnstile verification.
  const windowMs = closeWindowMs(env);
  if (windowMs > 0 && isClosed(windowMs, await lastActivity(env, url), payload.published || null)) {
    return json({ error: "comments closed" }, 403);
  }

  const ip = request.headers.get("cf-connecting-ip");
  if (!payload.token || !(await verifyTurnstile(env.TURNSTILE_SECRET, payload.token, ip))) {
    return json({ error: "captcha verification failed" }, 403);
  }

  const ua = request.headers.get("user-agent") || "";
  const spam = await isSpam(env, author, email, body, ip, ua);

  await env.COMMENTS_DB.prepare(
    `INSERT INTO comments
       (id, url, author, email, body, status, created_at, ip_hash, user_agent, avatar_hash)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  ).bind(
    crypto.randomUUID(),
    url,
    author,
    email || null,
    body,
    spam ? "spam" : "pending",
    new Date().toISOString(),
    // Store the IP hash only when a salt is configured: an unsalted sha256(ip)
    // over the 2^32 IPv4 space is trivially precomputable (reversible), which
    // would defeat the "raw IP is never recoverable" guarantee. No salt → store
    // nothing rather than a false-safe hash.
    ip && env.COMMENTS_IP_SALT ? await sha256hex(env.COMMENTS_IP_SALT + ip) : null,
    ua,
    email ? await sha256hex(email.toLowerCase()) : null,
  ).run();

  // Never reveal the spam verdict to the submitter — a spammer must not learn
  // they were filtered. Both paths look like "thanks, awaiting review".
  return json({ ok: true, status: "pending" }, 201);
};
