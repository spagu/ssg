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
} from "./_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const url = normaliseURL(new URL(request.url).searchParams.get("url"));
  if (!url) return json({ error: "a valid ?url= is required" }, 400);

  const order = env.COMMENTS_ORDER === "oldest" ? "ASC" : "DESC";
  const { results } = await env.COMMENTS_DB.prepare(
    `SELECT id, author, body, created_at, avatar_hash
       FROM comments WHERE url = ? AND status = 'approved'
       ORDER BY created_at ${order} LIMIT 500`,
  ).bind(url).all<CommentRow>();

  return json({ url, count: results.length, comments: results });
};

interface Body {
  url?: string;
  author?: string;
  email?: string;
  body?: string;
  token?: string; // Turnstile response
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  let payload: Body;
  try {
    payload = (await request.json()) as Body;
  } catch {
    return json({ error: "invalid JSON" }, 400);
  }

  const url = normaliseURL(payload.url);
  const author = (payload.author || "").trim().slice(0, 80);
  const email = (payload.email || "").trim().slice(0, 254);
  const body = (payload.body || "").trim().slice(0, 5000);
  if (!url || !author || !body) {
    return json({ error: "url, author and body are required" }, 422);
  }

  const ip = request.headers.get("cf-connecting-ip");
  if (!env.TURNSTILE_SECRET) return json({ error: "comments not configured" }, 503);
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
    ip ? await sha256hex((env.COMMENTS_IP_SALT || "") + ip) : null,
    ua,
    email ? await sha256hex(email.toLowerCase()) : null,
  ).run();

  // Never reveal the spam verdict to the submitter — a spammer must not learn
  // they were filtered. Both paths look like "thanks, awaiting review".
  return json({ ok: true, status: "pending" }, 201);
};
