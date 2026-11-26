// Fixtures for #460: a campaign in a given state with `sent` ledger rows, and
// Resend events that carry (or not) the campaign tag.
import { CAMPAIGN_TAG } from "../src/bounce-attribution.ts";
import { addCampaign } from "./engine-fixtures.ts";
import type { FakeD1 } from "./fakes.ts";
import { hook, svixHeaders } from "./hook-fixtures.ts";

// sendingCampaign: a campaign in `status` with `sent` ledger rows and a plan row.
export function campaignWithSends(d1: FakeD1, status: string, sent: number, fields: Record<string, string | null> = {}): string {
  const id = addCampaign(d1, { status, started_at: "2026-11-01T00:00:00.000Z", ...fields });
  const at = "2026-11-01T00:00:00.000Z";
  d1.sqlite.prepare("INSERT INTO send_plans (campaign_id, created_at, updated_at) VALUES (?, ?, ?)").run(id, at, at);
  const add = d1.sqlite.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'sent', 1, ?)");
  for (let i = 0; i < sent; i++) add.run(id, `s-${String(i).padStart(4, "0")}`, at);
  return id;
}

// addSent adds `n` more `sent` rows, numbered after `from`.
export function addSent(d1: FakeD1, id: string, from: number, n: number) {
  const add = d1.sqlite.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'sent', 1, ?)");
  for (let i = from; i < from + n; i++) add.run(id, `s-${String(i).padStart(4, "0")}`, "2026-11-01T00:00:00.000Z");
}

// tagged builds a Resend event; `tags` replaces the data.tags object.
export const tagged = (type: string, to: string, campaignId: unknown, bounce?: Record<string, unknown>, tags?: unknown) =>
  JSON.stringify({
    type,
    created_at: "2026-11-13T00:00:00.000Z",
    data: { email_id: "e1", to: [to], tags: tags ?? { [CAMPAIGN_TAG]: campaignId, category: "newsletter" }, ...(bounce ? { bounce } : {}) },
  });

export const HARD = { type: "Permanent", subType: "General", message: "550 5.1.1 user unknown" };

// resend POSTs a signed body to the Resend hook; `id` is the svix-id.
export async function resend(env: unknown, body: string, id?: string) {
  const res = await hook(env, "resend", body, await svixHeaders(body, { id }));
  return { status: res.status, body: await res.json() };
}

export const counts = (d1: FakeD1, id: string) =>
  Object.fromEntries(
    (d1.sqlite.prepare("SELECT source, kind, count FROM campaign_bounces WHERE campaign_id = ? ORDER BY source, kind").all(id) as { source: string; kind: string; count: number }[]).map(
      (r) => [`${r.source}:${r.kind}`, Number(r.count)],
    ),
  );

export const statusOf = (d1: FakeD1, id: string) => (d1.sqlite.prepare("SELECT status FROM campaigns WHERE id = ?").get(id) as { status: string }).status;
export const planErrorOf = (d1: FakeD1, id: string) => (d1.sqlite.prepare("SELECT error FROM send_plans WHERE campaign_id = ?").get(id) as { error: string | null }).error;
