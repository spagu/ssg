// An invented, believable dataset for the panel's local preview
// (test/dev-server.mjs) and its screenshots: about 1 800 subscribers over a
// year in five languages, campaigns in every state, suppressions of every
// reason, a dead letter. Deterministic (seeded PRNG), example.com addresses
// only, dates relative to `now`. Not used by the test suite.
import { ensureSchema } from "../src/schema.ts";
import { sha256hex } from "../src/lib.ts";
import type { FakeD1 } from "./fakes.ts";

const DAY = 86_400_000;

// mulberry32: a tiny seeded PRNG, so every run draws the same people.
function prng(seed: number) {
  return () => {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const FIRST = ["anna", "ben", "chloe", "dev", "eva", "farid", "greta", "hugo", "isha", "jan", "kasia", "lars", "mila", "nico", "ola", "priya", "quentin", "rosa", "sam", "tomasz", "uma", "vik", "wiebke", "xavier", "yara", "zofia", "arjun", "lea", "marek", "noor"];
const LAST = ["nowak", "meyer", "dubois", "sharma", "kowalska", "schmidt", "martin", "patel", "lewis", "fischer", "bernard", "rao", "wojcik", "weber", "laurent", "iyer"];
const DOMAINS = ["example.com", "example.org", "example.net", "mail.example.com", "post.example.org"];
const LANGS: [string, number][] = [["en", 0.42], ["pl", 0.2], ["de", 0.16], ["fr", 0.13], ["hi", 0.09]];
const TAGS = ["news", "offers", "weekly", "events", "product-updates", "recipes"];

const iso = (t: number) => new Date(t).toISOString();
const uuid = (r: () => number) =>
  "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const v = Math.floor(r() * 16);
    return (c === "x" ? v : (v & 3) | 8).toString(16);
  });

interface Person { id: string; email: string; status: string; language: string; tags: string; created: number; unsub: number | null; confirmSent: number | null; consent: string }

export interface DemoIds { sending: string; paused: string; uncertain: string; clicks: string; draft: string; scheduled: string }

export async function seedDemo(env: { NEWSLETTER_DB: FakeD1 }, now = Date.now()): Promise<DemoIds> {
  await ensureSchema(env as never);
  const r = prng(20261113);
  const db = env.NEWSLETTER_DB.sqlite;
  const pick = <T>(list: T[]) => list[Math.floor(r() * list.length)];
  const lang = () => {
    let x = r();
    for (const [l, w] of LANGS) if ((x -= w) < 0) return l;
    return "en";
  };

  // A year of sign-ups, growing (more recent days are busier) with spikes
  // after campaigns and a slow summer.
  const people: Person[] = [];
  const used = new Set<string>();
  for (let i = 0; people.length < 1800; i++) {
    const age = Math.floor(365 * Math.pow(r(), 0.72)); // days ago, skewed to recent
    const month = new Date(now - age * DAY).getUTCMonth();
    if ((month === 6 || month === 7) && r() < 0.35) continue;
    let email = `${pick(FIRST)}.${pick(LAST)}${r() < 0.3 ? Math.floor(r() * 90 + 10) : ""}@${pick(DOMAINS)}`;
    if (used.has(email)) email = email.replace("@", `${i}@`);
    used.add(email);
    const created = now - age * DAY - Math.floor(r() * DAY);
    const tags = TAGS.filter(() => r() < 0.28).join(",");
    people.push({ id: uuid(r), email, status: "confirmed", language: lang(), tags, created, unsub: null, confirmSent: created + 60_000, consent: "I want the newsletter. I can unsubscribe at any time with the link in every mail." });
  }
  // Pending: an import of paper sign-up sheets, not confirmed yet.
  for (let i = 0; i < 46; i++) {
    const p = people[i * 13];
    Object.assign(p, { status: "pending", confirmSent: null, consent: "Signed the paper list at the 2025 autumn meetup (imported)", created: now - 3 * DAY - i * 3600_000 });
  }

  // Campaigns, oldest first: [daysAgo, subject, state, language segment].
  const C = (daysAgo: number) => now - daysAgo * DAY;
  type Camp = { id: string; subject: string; status: string; start: number | null; segment: string; body: string; clicks?: boolean; uncertain?: number; failed?: number; share?: number; error?: string };
  const body = (title: string) =>
    `# ${title}\n\nHello! Here is what changed on the site this month.\n\n- A new [recipe collection]({{site_url}}/recipes/)\n- Notes from the [autumn meetup]({{site_url}}/blog/autumn-meetup/)\n- Opening hours over the holidays\n\nThank you for reading,  \nThe team`;
  const seg = (langs: string[] = [], tags: string[] = []) => JSON.stringify({ languages: langs, tags, tagMode: "any" });
  const camps: Camp[] = [
    { id: uuid(r), subject: "Spring notes: what we are cooking", status: "sent", start: C(300), segment: seg(), body: body("Spring notes"), failed: 4 },
    { id: uuid(r), subject: "Wiosenne nowości w kuchni", status: "sent", start: C(205), segment: seg(["pl"]), body: body("Wiosenne nowości"), clicks: true },
    { id: uuid(r), subject: "Summer break and the new opening hours", status: "sent", start: C(120), segment: seg(), body: body("Summer break"), failed: 9 },
    { id: uuid(r), subject: "Autumn meetup: photos and recordings", status: "sent", start: C(45), segment: seg([], ["events"]), body: body("Autumn meetup"), uncertain: 3, failed: 2 },
    { id: uuid(r), subject: "Herbstrezepte für lange Abende", status: "cancelled", start: C(30), segment: seg(["de"]), body: body("Herbstrezepte"), share: 0.4 },
    { id: uuid(r), subject: "November digest: twelve new recipes", status: "sent", start: C(12), segment: seg(), body: body("November digest"), clicks: true, failed: 6 },
    { id: uuid(r), subject: "Offres de fin d'année", status: "paused", start: C(2), segment: seg(["fr"]), body: body("Offres"), share: 0.35, error: "bounces: 6 of 10 messages in one batch were refused as unknown mailboxes; nothing was suppressed — check the list or the server before resuming" },
    { id: uuid(r), subject: "Holiday opening hours", status: "sending", start: now - 9 * 60_000, segment: seg(["en"]), body: body("Holiday opening hours"), share: 0.3, clicks: true },
    { id: uuid(r), subject: "Winter menu preview", status: "scheduled", start: null, segment: seg([], ["news", "recipes"]), body: body("Winter menu") },
    { id: uuid(r), subject: "New year plans", status: "draft", start: null, segment: seg(), body: "Write the new year's plans here.\n\nRead more on {{site_url}}" },
  ];

  // Unsubscribes, credited to a recent send: ~6% of confirmed people leave a
  // few days after a campaign that reached them.
  const confirmed = people.filter((p) => p.status === "confirmed");
  const sent = camps.filter((c) => c.start !== null && c.status !== "draft");
  for (const p of confirmed) {
    if (r() > 0.065) continue;
    const after = sent.filter((c) => (c.start as number) > p.created + DAY && c.status !== "sending");
    const c = after.length ? pick(after) : null;
    const at = c ? (c.start as number) + Math.floor(r() * 4 * DAY) + 3600_000 : p.created + Math.floor(r() * 20 * DAY);
    if (at < now) Object.assign(p, { status: "unsubscribed", unsub: at });
  }

  const insP = db.prepare(
    `INSERT INTO subscribers (id, email, status, language, source_page, tags, consent_text, consent_at, token, created_at, updated_at, confirmed_at, unsubscribed_at, confirm_sent_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  );
  db.exec("BEGIN");
  for (const p of people) {
    const src = pick(["/", "/blog/", "/recipes/", "/pl/", "/de/blog/", "/fr/"]);
    insP.run(p.id, p.email, p.status, p.language, src, p.tags, p.consent, iso(p.created), uuid(r).replace(/-/g, "") + "abcdefghij1", iso(p.created), iso(p.unsub ?? p.created), p.status === "pending" ? null : iso(p.created + 120_000), p.unsub ? iso(p.unsub) : null, p.confirmSent ? iso(p.confirmSent) : null);
  }
  db.exec("COMMIT");

  const insC = db.prepare(
    `INSERT INTO campaigns (id, subject, body, body_format, segment, status, scheduled_at, started_at, finished_at, total_count, sent_count, failed_count, source_post, created_at, updated_at)
     VALUES (?, ?, ?, 'markdown', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  );
  const insS = db.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, error, claimed_at, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?)");
  const insL = db.prepare("INSERT INTO click_links (campaign_id, link_id, url, clicks, created_at) VALUES (?, ?, ?, ?, ?)");
  const insPlan = db.prepare("INSERT INTO send_plans (campaign_id, cursor, planned_all, error, created_at, updated_at) VALUES (?, ?, 0, ?, ?, ?)");
  const insB = db.prepare("INSERT INTO send_batches (campaign_id, after_id, until_id, status, created_at, updated_at) VALUES (?, '', ?, 'done', ?, ?)");
  db.exec("BEGIN");
  for (const c of camps) {
    const s = JSON.parse(c.segment) as { languages: string[]; tags: string[] };
    if (c.start === null) {
      const at = c.status === "scheduled" ? iso(now + 3 * DAY) : null;
      insC.run(c.id, c.subject, c.body, c.segment, c.status, at, null, null, 0, 0, 0, c.status === "draft" ? null : "https://example.com/blog/winter-menu/", iso(now - 2 * DAY), iso(now - 3600_000));
      continue;
    }
    const start = c.start;
    // Recipients: confirmed at the start (or unsubscribed later), in segment, by id.
    const reach = people
      .filter((p) => p.status !== "pending" && p.created < start && (!p.unsub || p.unsub > start))
      .filter((p) => !s.languages.length || s.languages.includes(p.language))
      .filter((p) => !s.tags.length || s.tags.some((t) => p.tags.split(",").includes(t)))
      .sort((a, b) => (a.id < b.id ? -1 : 1));
    const upto = Math.floor(reach.length * (c.share ?? 1));
    let sentN = 0;
    let failedN = 0;
    reach.slice(0, upto).forEach((p, i) => {
      const at = start + i * 1000;
      const fail = i % 97 === 5 && failedN < (c.failed ?? 0);
      const unsure = i % 53 === 7 && i < 53 * (c.uncertain ?? 0);
      if (fail) failedN++;
      else if (!unsure) sentN++;
      insS.run(c.id, p.id, fail ? "failed" : unsure ? "uncertain" : "sent", 1, fail ? "smtp 550 at RCPT: 5.1.1 mailbox unavailable" : null, iso(at), fail || unsure ? null : iso(at + 800));
    });
    const finished = c.status === "sent" ? iso(start + upto * 1000 + 60_000) : null;
    insC.run(c.id, c.subject, c.body, c.segment, c.status, iso(start), iso(start), finished, reach.length, sentN, failedN, null, iso(start - 2 * DAY), iso(start));
    if (c.status === "sending" || c.status === "paused") {
      const cursor = reach[upto - 1].id;
      insPlan.run(c.id, cursor, c.error ?? null, iso(start), iso(start));
      insB.run(c.id, cursor, iso(start), iso(start));
    }
    if (c.clicks) {
      const links = ["/recipes/", "/blog/autumn-meetup/", "/opening-hours/", "/recipes/pierogi/"];
      for (const [k, path] of links.entries()) {
        const url = `https://example.com${path}`;
        insL.run(c.id, (await sha256hex(url)).slice(0, 16), url, Math.floor(sentN * (0.09 / (k + 1)) + 2), iso(start));
      }
    }
  }
  db.exec("COMMIT");

  // Suppressions of every reason across the year, mostly addresses that
  // bounced and so never stayed on the list.
  const insX = db.prepare("INSERT OR IGNORE INTO suppressions (email_hash, email, reason, count, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)");
  const plan: [string, number, string][] = [["hard", 34, "smtp"], ["soft", 11, "smtp"], ["complaint", 6, "resend"], ["manual", 9, "admin"], ["manual", 4, "erasure"]];
  const rows: unknown[][] = [];
  let n = 0;
  for (const [reason, count, source] of plan) {
    for (let i = 0; i < count; i++) {
      const email = `bounce${++n}.${pick(LAST)}@${pick(DOMAINS)}`;
      const at = now - Math.floor(365 * Math.pow(r(), 1.6)) * DAY - Math.floor(r() * DAY);
      rows.push([await sha256hex(email), source === "erasure" ? null : email, reason, reason === "soft" ? 3 : 1, source, iso(at), iso(at)]);
    }
  }
  db.exec("BEGIN");
  for (const row of rows) insX.run(...(row as never[]));
  db.prepare("INSERT INTO soft_bounces (email_hash, email, count, source, detail, updated_at) VALUES (?, ?, ?, 'smtp', ?, ?)").run(await sha256hex("full.inbox@example.org"), "full.inbox@example.org", 2, "452 4.2.2 mailbox full", iso(now - 5 * DAY));
  db.prepare("INSERT INTO send_dead_letters (id, campaign_id, job, attempts, received_at) VALUES (?, ?, ?, 6, ?)").run(uuid(r), camps[5].id, JSON.stringify({ type: "newsletter.send-batch", campaignId: camps[5].id, after: "", until: "" }), iso(now - 11 * DAY));
  db.exec("COMMIT");

  const id = (status: string) => (camps.find((c) => c.status === status) as Camp).id;
  return { sending: id("sending"), paused: id("paused"), uncertain: camps[3].id, clicks: camps[5].id, draft: id("draft"), scheduled: id("scheduled") };
}
