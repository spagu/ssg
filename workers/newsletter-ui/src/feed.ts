// Campaign draft from a post (GO-130): list the site feed's recent posts, and
// turn one into a DRAFT — subject = title, body = excerpt + link, segment
// language = the post's language. Nothing here sends or schedules; there is
// no trigger on publish. campaigns.source_post remembers the post's guid so a
// second campaign from the same post needs an explicit confirmation.

import { createDraft } from "./campaign-crud";
import { MAX_SUBJECT, readJson } from "./campaign-model";
import { fetchFeed } from "./feed-fetch";
import { FeedError, excerpt, parseFeed } from "./feed-parse";
import type { FeedPost } from "./feed-parse";
import type { Env } from "./lib";
import { json } from "./lib";
import type { RouteContext } from "./router";

export const MAX_POSTS = 50; // what the panel lists; ssg feeds hold the newest posts first
export const EXCERPT_CHARS = 500;
const MAX_GUID = 2000;

type Linked = { id: string; status: string };

// loadPosts fetches and parses the feed; a failure is the JSON answer to give.
async function loadPosts(env: Env): Promise<{ posts: FeedPost[]; url: string } | Response> {
  const got = await fetchFeed(env);
  if (!got.ok) return json({ error: got.error, detail: got.detail }, got.status);
  try {
    return { posts: parseFeed(got.text, got.url).slice(0, MAX_POSTS), url: got.url };
  } catch (e) {
    // parseFeed throws only FeedError; its message says what was refused.
    return json({ error: "invalid_feed", detail: (e as FeedError).message }, 502);
  }
}

// campaignsFor maps each guid to its newest campaign, in one query.
async function campaignsFor(env: Env, guids: string[]): Promise<Map<string, Linked>> {
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT id, status, source_post FROM campaigns
      WHERE source_post IN (SELECT value FROM json_each(?)) ORDER BY created_at ASC`,
  )
    .bind(JSON.stringify(guids))
    .all<Linked & { source_post: string }>();
  return new Map(results.map((r) => [r.source_post, { id: r.id, status: r.status }])); // later rows win: newest
}

// GET /api/feed/posts → {items: [post + campaign: {id, status} | null], nextCursor: null, feedUrl}
export async function listFeedPosts({ env }: RouteContext): Promise<Response> {
  const loaded = await loadPosts(env);
  if (loaded instanceof Response) return loaded;
  const linked = await campaignsFor(env, loaded.posts.map((p) => p.guid));
  const items = loaded.posts.map((p) => ({ ...p, summary: excerpt(p.summary, EXCERPT_CHARS), campaign: linked.get(p.guid) ?? null }));
  return json({ items, nextCursor: null, feedUrl: loaded.url });
}

// Markdown that must stay text: the excerpt and the title are the site's
// words, not markup. Braces are escaped too, so "{{x}}" in a post is shown,
// never read as a variable.
const MD_SPECIAL = /[\\`*_[\]<>#~{}!|]/g;
export function escapeMarkdown(s: string): string {
  return s.replace(MD_SPECIAL, "\\$&").replace(/^([-+]|\d+\.)(?=\s)/, (m) => m.replace(/([-+.])$/, "\\$1"));
}

const linkTarget = (url: string): string => url.replace(/[()<>\s]/g, (c) => "%" + c.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0"));

// draftFields are the API fields of a draft made from a post.
export function draftFields(p: FeedPost): Record<string, unknown> {
  const summary = excerpt(p.summary, EXCERPT_CHARS);
  const link = p.link ? `[${escapeMarkdown(p.title || p.link)}](${linkTarget(p.link)})` : "";
  return {
    subject: excerpt(p.title, MAX_SUBJECT),
    body: [summary && escapeMarkdown(summary), link].filter(Boolean).join("\n\n"),
    segment: { languages: p.language ? [p.language] : [], tags: [], tagMode: "any" },
  };
}

// POST /api/campaigns/from-post {guid, confirm?} → 201 the draft;
// 409 already_exists {id} when a campaign was made from this post before.
export async function campaignFromPost(ctx: RouteContext): Promise<Response> {
  const b = await readJson(ctx.request);
  if (!b || typeof b.guid !== "string" || !b.guid || b.guid.length > MAX_GUID) {
    return json({ error: "invalid_input", detail: "guid must be the post's guid from GET /api/feed/posts" }, 400);
  }
  if (b.confirm !== undefined && typeof b.confirm !== "boolean") return json({ error: "invalid_input", detail: "confirm must be true or false" }, 400);
  const guid = b.guid;
  if (b.confirm !== true) {
    const existing = (await campaignsFor(ctx.env, [guid])).get(guid);
    if (existing) {
      return json({ error: "already_exists", detail: "a campaign was already made from this post; send confirm: true for another", id: existing.id }, 409);
    }
  }
  const loaded = await loadPosts(ctx.env);
  if (loaded instanceof Response) return loaded;
  const post = loaded.posts.find((p) => p.guid === guid);
  if (!post) return json({ error: "post_not_found", detail: "no post with this guid in the feed's recent posts" }, 404);
  return createDraft(ctx, draftFields(post), { source_post: guid });
}
