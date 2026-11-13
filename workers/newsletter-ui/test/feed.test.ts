// GO-130 routes: GET /api/feed/posts and POST /api/campaigns/from-post through
// the whole pipeline on real SQL — the draft, the duplicate guard, the same
// validation as a hand-written draft, and feed failures that create nothing.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { draftFields, escapeMarkdown } from "../src/feed.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { call, mailEnv } from "./campaign-fixtures.ts";
import { ATOM, RSS, feedFetch } from "./feed-fixtures.ts";

const FEED = "https://example.com/feed.xml";
const FISH = "tag:example.com,2026:fish";

async function api(env: ReturnType<typeof mailEnv>, method: string, path: string, body?: unknown) {
  const init: RequestInit & { auth: string } = { method, auth: PASSWORD };
  if (body !== undefined) init.body = typeof body === "string" ? body : JSON.stringify(body);
  const res = await handleFetch(req("/newsletter-admin/api" + path, init), env as never, fakeCtx() as never);
  return { status: res.status, body: await res.json() };
}

// withFeed runs fn with the fetch answering `body` at the default feed URL.
async function withFeed<T>(body: string, fn: () => Promise<T>, headers = { "content-type": "application/atom+xml" }): Promise<T> {
  const f = feedFetch({ [FEED]: { body, headers } });
  try {
    return await fn();
  } finally {
    f.restore();
  }
}

const campaignCount = (env: ReturnType<typeof mailEnv>) => Number((env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM campaigns").get() as { n: number }).n);

test("posts: the feed's items with an excerpt and the campaign made from each", async () => {
  const env = mailEnv();
  await withFeed(ATOM, async () => {
    const r = await api(env, "GET", "/feed/posts");
    assert.equal(r.status, 200);
    assert.equal(r.body.feedUrl, FEED);
    assert.equal(r.body.nextCursor, null);
    assert.deepEqual(r.body.items.map((p: { guid: string }) => p.guid), [FISH, "https://example.com/posts/tea/", "urn:x:bare"]);
    assert.deepEqual(r.body.items[0], {
      guid: FISH,
      title: "Fish & chips now",
      link: "https://example.com/posts/fish/",
      summary: "Crispy & hot.",
      published: "2026-11-10T08:00:00.000Z",
      language: "pl",
      campaign: null,
    });
    const made = await api(env, "POST", "/campaigns/from-post", { guid: FISH });
    assert.equal(made.status, 201);
    const again = await api(env, "GET", "/feed/posts");
    assert.deepEqual(again.body.items[0].campaign, { id: made.body.id, status: "draft" });
    assert.equal(again.body.items[1].campaign, null);
  });
});

test("from-post: a draft with title, excerpt + link, the post's language and source_post — never sent", async () => {
  const env = mailEnv();
  const r = await withFeed(ATOM, () => api(env, "POST", "/campaigns/from-post", { guid: FISH }));
  assert.equal(r.status, 201);
  assert.equal(r.body.status, "draft");
  assert.equal(r.body.subject, "Fish & chips now");
  assert.equal(r.body.body, "Crispy & hot.\n\n[Fish & chips now](https://example.com/posts/fish/)");
  assert.equal(r.body.bodyFormat, "markdown");
  assert.deepEqual(r.body.segment, { languages: ["pl"], tags: [], tagMode: "any" });
  assert.equal(r.body.sourcePost, FISH);
  assert.equal(r.body.scheduledAt, null);
  assert.equal(env.NEWSLETTER_QUEUE.sent.length, 0);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM campaign_sends").get()?.n, 0);
});

test("from-post: the second campaign from a post needs confirm: true", async () => {
  const env = mailEnv();
  await withFeed(RSS, async () => {
    const first = await api(env, "POST", "/campaigns/from-post", { guid: "post-1" });
    assert.equal(first.status, 201);
    assert.deepEqual(first.body.segment.languages, ["fr"]);
    const dup = await api(env, "POST", "/campaigns/from-post", { guid: "post-1" });
    assert.equal(dup.status, 409);
    assert.deepEqual([dup.body.error, dup.body.id], ["already_exists", first.body.id]);
    const no = await api(env, "POST", "/campaigns/from-post", { guid: "post-1", confirm: false });
    assert.equal(no.status, 409);
    const yes = await api(env, "POST", "/campaigns/from-post", { guid: "post-1", confirm: true });
    assert.equal(yes.status, 201);
    assert.notEqual(yes.body.id, first.body.id);
    assert.equal(campaignCount(env), 2);
    const third = await api(env, "POST", "/campaigns/from-post", { guid: "post-1" });
    assert.equal(third.body.id, yes.body.id, "points at the newest campaign from the post");
  });
});

test("from-post: the draft passes the same validation and scheduling as a hand-written one", async () => {
  const env = mailEnv();
  await withFeed(ATOM, async () => {
    const fromPost = await api(env, "POST", "/campaigns/from-post", { guid: FISH });
    const posts = (await api(env, "GET", "/feed/posts")).body.items;
    const manual = await call(env, "POST", "", draftFields(posts[0]));
    assert.equal(manual.status, 201);
    for (const k of ["subject", "body", "bodyFormat", "segment", "textOverride", "fromEmail"]) {
      assert.deepEqual(fromPost.body[k], manual.body[k], k);
    }
    const preview = await call(env, "POST", `/${fromPost.body.id}/preview`, { language: "pl" });
    assert.match(preview.body.text, /Crispy & hot\.\n\nFish & chips now \(https:\/\/example\.com\/posts\/fish\/\)/);
    assert.deepEqual(preview.body.recipients, { total: 1, byLanguage: { pl: 1 } });
    const scheduled = await call(env, "POST", `/${fromPost.body.id}/schedule`, {});
    assert.equal(scheduled.status, 200);
    assert.equal(scheduled.body.status, "scheduled");
  });
});

test("from-post: a title the validator refuses creates nothing; a long title is shortened", async () => {
  const env = mailEnv();
  const feed = (title: string, summary = "") =>
    `<rss><channel><item><guid>g</guid><title>${title}</title><link>https://example.com/p/</link><description>${summary}</description></item></channel></rss>`;
  const bad = await withFeed(feed("Using {{handlebars}}"), () => api(env, "POST", "/campaigns/from-post", { guid: "g" }));
  assert.deepEqual([bad.status, bad.body.error], [400, "unknown_variable"]);
  assert.equal(campaignCount(env), 0);
  const long = await withFeed(feed("word ".repeat(80), "{{x}} *not bold* - [link](javascript:x)"), () =>
    api(env, "POST", "/campaigns/from-post", { guid: "g" }),
  );
  assert.equal(long.status, 201);
  assert.ok(long.body.subject.length <= 250 && long.body.subject.endsWith("…"));
  assert.match(long.body.body, /^\\\{\\\{x\\\}\\\} \\\*not bold\\\* - \\\[link\\\]\(javascript:x\)\n\n/);
  const preview = await call(env, "POST", `/${long.body.id}/preview`, {});
  assert.match(preview.body.text, /^\{\{x\}\} \*not bold\* - \[link\]\(javascript:x\)/);
});

test("from-post: input errors, an unknown guid, and feed failures create no draft", async () => {
  const env = mailEnv();
  for (const body of ["nope", {}, { guid: "" }, { guid: 5 }, { guid: "x".repeat(2001) }, { guid: "g", confirm: "yes" }]) {
    const r = await withFeed(ATOM, () => api(env, "POST", "/campaigns/from-post", body));
    assert.deepEqual([r.status, r.body.error], [400, "invalid_input"], JSON.stringify(body));
  }
  const missing = await withFeed(ATOM, () => api(env, "POST", "/campaigns/from-post", { guid: "not-in-feed" }));
  assert.deepEqual([missing.status, missing.body.error], [404, "post_not_found"]);
  const failures: [string, Record<string, string>, number, string][] = [
    ["<html>404</html>", { "content-type": "text/html" }, 502, "feed_content_type"],
    ["<rss><channel><item>", { "content-type": "application/rss+xml" }, 502, "invalid_feed"],
  ];
  for (const [body, headers, status, error] of failures) {
    for (const [method, path, payload] of [["GET", "/feed/posts", undefined], ["POST", "/campaigns/from-post", { guid: FISH }]] as const) {
      const r = await withFeed(body, () => api(env, method, path, payload), headers);
      assert.deepEqual([r.status, r.body.error], [status, error], `${method} ${path}`);
      assert.equal(typeof r.body.detail, "string");
    }
  }
  const unset = mailEnv({ NEWSLETTER_SITE_URL: undefined });
  const r = await api(unset, "GET", "/feed/posts");
  assert.deepEqual([r.status, r.body.error], [503, "no_feed_url"]);
  assert.equal(campaignCount(env), 0);
});

test("draftFields and escapeMarkdown: links, missing parts, list markers", () => {
  const post = { guid: "g", title: "", link: "https://example.com/a (b)/", summary: "", published: null, language: null };
  assert.deepEqual(draftFields(post), {
    subject: "",
    body: "[https://example.com/a (b)/](https://example.com/a%20%28b%29/)",
    segment: { languages: [], tags: [], tagMode: "any" },
  });
  assert.equal(draftFields({ ...post, link: "", summary: "Only text" }).body, "Only text");
  assert.equal(escapeMarkdown("- item"), "\\- item");
  assert.equal(escapeMarkdown("12. first"), "12\\. first");
  assert.equal(escapeMarkdown("+ x # y"), "\\+ x \\# y");
  assert.equal(escapeMarkdown("-dash"), "-dash");
});

test("auth refusal table: every GO-130 route needs auth; writes need the CSRF header", async () => {
  const env = mailEnv();
  const routes: [string, string][] = [
    ["GET", "/feed/posts"],
    ["POST", "/campaigns/from-post"],
    ["POST", "/subscribers/confirmations"],
    ["POST", "/subscribers/s3/confirmation"],
  ];
  const f = feedFetch({ [FEED]: { body: ATOM } });
  try {
    for (const [n, [method, path]] of routes.entries()) {
      const url = "/newsletter-admin/api" + path;
      assert.equal((await handleFetch(req(url, { method }), env as never, fakeCtx() as never)).status, 401, `${method} ${path}`);
      const wrong = await handleFetch(req(url, { method, auth: "wrong", ip: `198.51.100.${n + 50}` }), env as never, fakeCtx() as never);
      assert.equal(wrong.status, 401, `${method} ${path} wrong password`);
      if (method !== "GET") {
        const noCsrf = await handleFetch(req(url, { method, auth: PASSWORD, csrf: false }), env as never, fakeCtx() as never);
        assert.equal(noCsrf.status, 403, `${method} ${path} without CSRF`);
      }
    }
    assert.equal(f.calls.length, 0, "no refused request reached the feed");
  } finally {
    f.restore();
  }
  assert.equal(campaignCount(env), 0);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM subscribers WHERE confirm_sent_at IS NOT NULL").get()?.n, 0);
});
