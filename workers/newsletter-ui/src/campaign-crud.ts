// Campaign CRUD (GO-125): list, create, read, update (draft only), delete
// (draft or cancelled only), duplicate into a new draft.

import { fromRow, isInvalid, loadCampaign, readJson, validateInput } from "./campaign-model";
import type { Campaign, CampaignRow, Columns } from "./campaign-model";
import { isStatus } from "./campaign-state";
import { json } from "./lib";
import type { RouteContext } from "./router";

const notFound = () => json({ error: "not_found" }, 404);
const badJson = () => json({ error: "invalid_json", detail: "the body must be a JSON object" }, 400);

// The list omits the bodies: the panel shows a table, and a body can be 256 KB.
const summary = ({ body: _b, textOverride: _t, ...rest }: Campaign) => rest;

// Cursor = the last item's (created_at, id), base64url-encoded: opaque to the
// client, stable when rows are added in between.
const encodeCursor = (c: Campaign) => btoa(JSON.stringify([c.createdAt, c.id])).replace(/=+$/, "");
function decodeCursor(raw: string): [string, string] | null {
  try {
    const v = JSON.parse(atob(raw));
    return Array.isArray(v) && v.length === 2 && v.every((x) => typeof x === "string") ? (v as [string, string]) : null;
  } catch {
    return null;
  }
}

// GET /api/campaigns?status=&cursor=&limit= — newest first.
export async function listCampaigns({ env, url }: RouteContext): Promise<Response> {
  const status = url.searchParams.get("status");
  if (status !== null && !isStatus(status)) return json({ error: "invalid_input", detail: "unknown status" }, 400);
  const limit = Math.min(Math.max(Number.parseInt(url.searchParams.get("limit") ?? "", 10) || 50, 1), 100);
  const where: string[] = [];
  const binds: string[] = [];
  if (status) {
    where.push("status = ?");
    binds.push(status);
  }
  const rawCursor = url.searchParams.get("cursor");
  if (rawCursor) {
    const cur = decodeCursor(rawCursor);
    if (!cur) return json({ error: "invalid_input", detail: "invalid cursor" }, 400);
    where.push("(created_at < ? OR (created_at = ? AND id < ?))");
    binds.push(cur[0], cur[0], cur[1]);
  }
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT * FROM campaigns ${where.length ? "WHERE " + where.join(" AND ") : ""} ORDER BY created_at DESC, id DESC LIMIT ?`,
  )
    .bind(...binds, limit + 1)
    .all<CampaignRow>();
  const items = results.slice(0, limit).map(fromRow);
  const nextCursor = results.length > limit ? encodeCursor(items[items.length - 1]) : null;
  return json({ items: items.map(summary), nextCursor });
}

// insert writes a new draft from validated columns.
async function insert(ctx: RouteContext, cols: Columns): Promise<Response> {
  const now = new Date().toISOString();
  const id = crypto.randomUUID();
  const row: Columns = { subject: "", body: "", body_format: "markdown", segment: "{}", ...cols, id, status: "draft", created_at: now, updated_at: now };
  const names = Object.keys(row);
  await ctx.env.NEWSLETTER_DB.prepare(`INSERT INTO campaigns (${names.join(", ")}) VALUES (${names.map(() => "?").join(", ")})`)
    .bind(...names.map((n) => row[n]))
    .run();
  return json(await loadCampaign(ctx.env, id), 201);
}

// createDraft is the one way a new draft is made from API-shaped fields: the
// same validation for a hand-written draft and for one built from a post
// (GO-130). `extra` are columns set by the worker itself, never by the
// client (source_post).
export async function createDraft(ctx: RouteContext, b: Record<string, unknown>, extra: Columns = {}): Promise<Response> {
  const cols = validateInput(b);
  if (isInvalid(cols)) return json(cols, 400);
  if (!("segment" in cols)) cols.segment = JSON.stringify({ languages: [], tags: [], tagMode: "any" });
  return insert(ctx, { ...cols, ...extra });
}

// POST /api/campaigns — every field optional; a new campaign is a draft.
export async function createCampaign(ctx: RouteContext): Promise<Response> {
  const b = await readJson(ctx.request);
  return b ? createDraft(ctx, b) : badJson();
}

export async function getCampaign({ env, params }: RouteContext): Promise<Response> {
  const c = await loadCampaign(env, params.id);
  return c ? json(c) : notFound();
}

// PATCH /api/campaigns/:id — only a draft. The UPDATE itself is guarded by
// status = 'draft', so a campaign scheduled a moment ago is not edited.
export async function updateCampaign({ request, env, params }: RouteContext): Promise<Response> {
  const b = await readJson(request);
  if (!b) return badJson();
  const cols = validateInput(b);
  if (isInvalid(cols)) return json(cols, 400);
  const names = Object.keys(cols);
  const r = await env.NEWSLETTER_DB.prepare(
    `UPDATE campaigns SET ${[...names.map((n) => `${n} = ?`), "updated_at = ?"].join(", ")} WHERE id = ? AND status = 'draft'`,
  )
    .bind(...names.map((n) => cols[n]), new Date().toISOString(), params.id)
    .run();
  const c = await loadCampaign(env, params.id);
  if (!c) return notFound();
  if (r.meta.changes !== 1) return json({ error: "not_draft", detail: `a ${c.status} campaign cannot be edited` }, 409);
  return json(c);
}

// DELETE /api/campaigns/:id — a draft or a cancelled campaign, with its send
// records. A campaign that is scheduled, sending, paused or sent stays.
export async function deleteCampaign({ env, params }: RouteContext): Promise<Response> {
  const db = env.NEWSLETTER_DB;
  const [gone] = await db.batch([
    db.prepare("DELETE FROM campaigns WHERE id = ? AND status IN ('draft', 'cancelled')").bind(params.id),
    db.prepare("DELETE FROM campaign_sends WHERE campaign_id = ? AND NOT EXISTS (SELECT 1 FROM campaigns WHERE id = ?)").bind(params.id, params.id),
    // GO-129: its click links go with it (their redirects answer 404 from now on).
    db.prepare("DELETE FROM click_links WHERE campaign_id = ? AND NOT EXISTS (SELECT 1 FROM campaigns WHERE id = ?)").bind(params.id, params.id),
    // GO-127: the engine's plan and ranges for it (a cancelled campaign has them).
    db.prepare("DELETE FROM send_plans WHERE campaign_id = ? AND NOT EXISTS (SELECT 1 FROM campaigns WHERE id = ?)").bind(params.id, params.id),
    db.prepare("DELETE FROM send_batches WHERE campaign_id = ? AND NOT EXISTS (SELECT 1 FROM campaigns WHERE id = ?)").bind(params.id, params.id),
  ]);
  if (gone.meta.changes === 1) return new Response(null, { status: 204 });
  const c = await loadCampaign(env, params.id);
  return c ? json({ error: "not_deletable", detail: `a ${c.status} campaign cannot be deleted` }, 409) : notFound();
}

// POST /api/campaigns/:id/duplicate — content, sender and segment into a new
// draft, from a campaign in any state.
export async function duplicateCampaign(ctx: RouteContext): Promise<Response> {
  const c = await loadCampaign(ctx.env, ctx.params.id);
  if (!c) return notFound();
  return insert(ctx, {
    subject: c.subject,
    body: c.body,
    body_format: c.bodyFormat,
    text_override: c.textOverride,
    from_email: c.fromEmail,
    from_name: c.fromName,
    segment: JSON.stringify(c.segment),
  });
}
