// The click redirect (GO-129): GET|HEAD {base}/hooks/c/:campaign/:link.
//
// Public, like every /hooks/ route, and with no signature to check: the URL
// carries no person identifier and can only lead to a target an admin already
// put in that campaign — a forged or guessed URL either matches a stored
// (campaign, link) pair, and goes where the mail would have gone anyway, or is
// answered 404. It is never an open redirector: the target is read from
// click_links, never from the request.
//
// GET counts one click and answers 302. HEAD answers the same 302 without
// counting: link checkers and mail scanners probe with it. Many scanners GET
// as well, so the count stays a relative signal (see README).
// Nothing about the request (IP, User-Agent, cookies) is read or stored.

import { json } from "./lib";
import type { RouteContext } from "./router";

// Only the renderer writes click_links, and only http(s) links; the query
// refuses anything else too, so even a row written by hand cannot make this
// route redirect to another scheme — or count a click for it.
const STORED = "campaign_id = ? AND link_id = ? AND (url LIKE 'https://%' OR url LIKE 'http://%')";

export async function clickRedirect({ request, env, params }: RouteContext): Promise<Response> {
  const db = env.NEWSLETTER_DB;
  const row =
    request.method === "GET"
      ? await db
          .prepare(`UPDATE click_links SET clicks = clicks + 1 WHERE ${STORED} RETURNING url`)
          .bind(params.campaign, params.link)
          .first<{ url: string }>()
      : await db.prepare(`SELECT url FROM click_links WHERE ${STORED}`).bind(params.campaign, params.link).first<{ url: string }>();
  if (!row) return json({ error: "not_found" }, 404);
  return new Response(null, { status: 302, headers: { location: row.url } });
}
