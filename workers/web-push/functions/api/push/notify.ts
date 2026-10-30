// Cloudflare Pages Function: announce new feed items to every subscriber.
//   POST /api/push/notify   Authorization: Bearer <PUSH_KEY>
//
// Call it after each deploy (see the README for the CI step). It reads the
// site's feed, compares it with the last item it announced (kept in KV) and
// sends one notification per new item — at most PUSH_MAX_ITEMS, the newest.
//
// The first call only remembers the newest item, so switching the worker on
// does not announce the whole archive. The marker moves before anything is
// sent: a retried call after a partial failure must not notify twice.
//
// Every message is a request to a push service, so one call makes
// subscribers × items subrequests; mind your plan's limit per invocation.

import { intVar, json, timingSafeEqual } from "./_lib";
import type { Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { deleteSubscription, listSubscriptions } from "./_store";
import { excerpt, newItems, parseFeed } from "./_feed";
import type { FeedItem } from "./_feed";
import { sendPush } from "./_webpush";
import type { VapidConfig } from "./_webpush";

const LAST_ITEM = "last-item";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  if (!env.PUSH_KEY) return json({ error: "not_configured" }, 503);
  const header = request.headers.get("authorization") || "";
  const token = header.startsWith("Bearer ") ? header.slice(7) : "";
  if (!timingSafeEqual(token, env.PUSH_KEY)) return json({ error: "unauthorized" }, 401);
  if (!env.PUSH_DB || !env.PUSH_KV || !env.PUSH_SUBJECT || !env.PUSH_VAPID_PUBLIC_KEY || !env.PUSH_VAPID_PRIVATE_KEY) {
    return json({ error: "not_configured" }, 503);
  }

  const feedURL = new URL(env.PUSH_FEED_URL || "/feed.xml", request.url);
  const res = await fetch(feedURL.toString(), {
    headers: { accept: "application/atom+xml, application/rss+xml, application/feed+json, application/json;q=0.9, */*;q=0.1" },
  });
  if (!res.ok) return json({ error: "feed_unavailable", status: res.status }, 502);
  const items = parseFeed(await res.text(), res.headers.get("content-type") || "");
  if (!items.length) return json({ sent: 0, removed: 0, items: [] });

  const last = await env.PUSH_KV.get(LAST_ITEM);
  if (last !== items[0].id) await env.PUSH_KV.put(LAST_ITEM, items[0].id);
  if (last === null) return json({ sent: 0, removed: 0, items: [], primed: items[0].id });

  const fresh = newItems(items, last, intVar(env.PUSH_MAX_ITEMS, 3));
  if (!fresh.length) return json({ sent: 0, removed: 0, items: [] });

  await ensureSchema(env);
  const vapid: VapidConfig = {
    subject: env.PUSH_SUBJECT,
    publicKey: env.PUSH_VAPID_PUBLIC_KEY,
    privateKey: env.PUSH_VAPID_PRIVATE_KEY,
    ttl: intVar(env.PUSH_TTL, 86400),
  };
  const result = await deliver(env, fresh, feedURL, vapid);
  return json({ ...result, items: fresh.map((it) => ({ title: it.title, url: new URL(it.url, feedURL).toString() })) });
};

// deliver sends the items oldest first, so the newest notification ends up on
// top, and drops every subscription its push service says is gone (404/410).
async function deliver(env: Env, items: FeedItem[], base: URL, vapid: VapidConfig) {
  let subs = await listSubscriptions(env);
  let sent = 0;
  let failed = 0;
  const gone = new Set<string>();
  for (const it of [...items].reverse()) {
    const payload = { title: it.title || it.url, body: excerpt(it.summary), url: new URL(it.url, base).toString() };
    const statuses = await Promise.all(subs.map((s) => sendPush(s, payload, vapid).catch(() => 0)));
    statuses.forEach((status, i) => {
      if (status === 404 || status === 410) gone.add(subs[i].endpoint);
      else if (status >= 200 && status < 300) sent++;
      else failed++;
    });
    subs = subs.filter((s) => !gone.has(s.endpoint));
  }
  for (const endpoint of gone) await deleteSubscription(env, endpoint);
  return { sent, failed, removed: gone.size };
}
