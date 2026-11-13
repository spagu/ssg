// PATCH  {base}/api/subscribers/:id              — tags and language, nothing else
// POST   {base}/api/subscribers/:id/unsubscribe  — manual unsubscribe
// DELETE {base}/api/subscribers/:id              — GDPR erasure
//
// Status moves only by unsubscribing. An admin can never set `confirmed`: the
// consent is the subscriber's to give, and the record of it (consent_text,
// consent_at) is evidence, so it is not editable either.

import type { RouteContext } from "./router";
import { json } from "./lib";
import { normaliseLang, parseTags, readJSONObject } from "./input";
import { detailOf, findSubscriber, notFound } from "./subscribers";
import { suppressStatement } from "./suppress";

const EDITABLE = ["tags", "language"];

export async function patchSubscriber({ request, env, params }: RouteContext): Promise<Response> {
  const body = await readJSONObject(request);
  if (!body) return json({ error: "invalid_body", detail: "send a JSON object" }, 400);
  const keys = Object.keys(body);
  const refused = keys.filter((k) => !EDITABLE.includes(k));
  if (refused.length) {
    return json({ error: "field_not_editable", detail: `only tags and language can be changed; refused: ${refused.join(", ")}` }, 400);
  }
  if (!keys.length) return json({ error: "invalid_body", detail: "nothing to change: send tags and/or language" }, 400);

  const set: string[] = [];
  const args: unknown[] = [];
  if ("tags" in body) {
    const tags = parseTags(body.tags);
    if (!tags) return json({ error: "invalid_tags", detail: "at most 10 tags of 1-32 characters a-z 0-9 _ -" }, 400);
    set.push("tags = ?");
    args.push(tags.join(","));
  }
  if ("language" in body) {
    // null or "" clears the language; anything else must be a valid tag.
    const raw = body.language;
    const lang = raw === null || raw === "" ? null : normaliseLang(raw);
    if (lang === null && raw !== null && raw !== "") {
      return json({ error: "invalid_language", detail: "language must be a tag like en or pt-BR, or null" }, 400);
    }
    set.push("language = ?");
    args.push(lang);
  }
  const res = await env.NEWSLETTER_DB.prepare(`UPDATE subscribers SET ${set.join(", ")}, updated_at = ? WHERE id = ?`)
    .bind(...args, new Date().toISOString(), params.id)
    .run();
  if (!res.meta.changes) return notFound();
  return json(await detailOf(env, (await findSubscriber(env, params.id)) as Record<string, unknown>));
}

// unsubscribeSubscriber has the same field effects as the public unsubscribe
// link in the `newsletter` template: status, unsubscribed_at, updated_at, and
// an already unsubscribed row keeps its original unsubscribed_at.
export async function unsubscribeSubscriber({ env, params }: RouteContext): Promise<Response> {
  const at = new Date().toISOString();
  const res = await env.NEWSLETTER_DB.prepare(
    `UPDATE subscribers SET status = 'unsubscribed', unsubscribed_at = ?, updated_at = ?
      WHERE id = ? AND status != 'unsubscribed'`,
  )
    .bind(at, at, params.id)
    .run();
  const row = await findSubscriber(env, params.id);
  if (!row) return notFound();
  return json({ ok: true, changed: res.meta.changes > 0, subscriber: await detailOf(env, row) });
}

// eraseSubscriber deletes the person and their send history in one batch (one
// transaction), and leaves behind only a hash-only suppression, so an import
// cannot bring the address back. After it the address exists in clear text
// nowhere in the database.
export async function eraseSubscriber({ env, params }: RouteContext): Promise<Response> {
  const row = await findSubscriber(env, params.id);
  if (!row) return notFound();
  const db = env.NEWSLETTER_DB;
  await db.batch([
    db.prepare("DELETE FROM campaign_sends WHERE subscriber_id = ?").bind(params.id),
    db.prepare("DELETE FROM subscribers WHERE id = ?").bind(params.id),
    await suppressStatement(env, { email: row.email as string, reason: "manual", source: "erasure", keepEmail: false }),
  ]);
  return json({ ok: true, erased: params.id });
}
