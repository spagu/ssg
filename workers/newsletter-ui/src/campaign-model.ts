// A campaign as the API speaks it (camelCase) and as the campaigns table
// stores it (snake_case), plus validation of what an admin may write.

import type { Env } from "./lib";
import { isAddress } from "./mail/_mime";
import { unknownVariables } from "./render";
import type { CampaignContent } from "./render";
import { parseSegment } from "./segment";
import type { Segment } from "./segment";
import type { Status } from "./campaign-state";

export interface Campaign extends CampaignContent {
  id: string;
  fromEmail: string | null;
  fromName: string | null;
  segment: Segment;
  status: Status;
  scheduledAt: string | null;
  startedAt: string | null;
  finishedAt: string | null;
  totalCount: number;
  sentCount: number;
  failedCount: number;
  sourcePost: string | null;
  createdAt: string;
  updatedAt: string;
}

export type CampaignRow = Record<string, unknown>;

// A stored segment is written only through parseSegment; should one ever be
// unreadable, it narrows to nobody (a tag the subscribers table cannot hold)
// rather than widening to every confirmed subscriber.
const NOBODY: Segment = { languages: [], tags: ["#unreadable-segment"], tagMode: "any" };

function storedSegment(raw: unknown): Segment {
  try {
    const s = parseSegment(JSON.parse(String(raw)));
    return typeof s === "string" ? NOBODY : s;
  } catch {
    return NOBODY;
  }
}

export function fromRow(r: CampaignRow): Campaign {
  return {
    id: r.id as string,
    subject: r.subject as string,
    body: r.body as string,
    bodyFormat: r.body_format === "html" ? "html" : "markdown",
    textOverride: (r.text_override as string | null) ?? null,
    fromEmail: (r.from_email as string | null) ?? null,
    fromName: (r.from_name as string | null) ?? null,
    segment: storedSegment(r.segment),
    status: r.status as Status,
    scheduledAt: (r.scheduled_at as string | null) ?? null,
    startedAt: (r.started_at as string | null) ?? null,
    finishedAt: (r.finished_at as string | null) ?? null,
    totalCount: Number(r.total_count),
    sentCount: Number(r.sent_count),
    failedCount: Number(r.failed_count),
    sourcePost: (r.source_post as string | null) ?? null,
    createdAt: r.created_at as string,
    updatedAt: r.updated_at as string,
  };
}

export async function loadCampaign(env: Env, id: string): Promise<Campaign | null> {
  const row = await env.NEWSLETTER_DB.prepare("SELECT * FROM campaigns WHERE id = ?").bind(id).first<CampaignRow>();
  return row ? fromRow(row) : null;
}

// readJson returns the parsed object body, or null when it is not one.
export async function readJson(request: Request): Promise<Record<string, unknown> | null> {
  try {
    const v = await request.json();
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

export const MAX_BODY = 256 * 1024; // characters; a mail far beyond this gets clipped by clients
export const MAX_SUBJECT = 250;

export type Invalid = { error: string; detail: string };
export type Columns = Record<string, string | null>;

const str = (v: unknown, nullable: boolean): v is string | null => typeof v === "string" || (nullable && v === null);

// validateInput checks the writable fields present in `b` and returns the
// columns to write. Every text field is checked for unknown {{variables}} here,
// on save, so a draft can never hold one.
export function validateInput(b: Record<string, unknown>): Columns | Invalid {
  const bad = (detail: string, error = "invalid_input"): Invalid => ({ error, detail });
  const cols: Columns = {};
  if ("subject" in b) {
    if (typeof b.subject !== "string" || b.subject.length > MAX_SUBJECT || /[\r\n]/.test(b.subject)) {
      return bad(`subject must be one line of at most ${MAX_SUBJECT} characters`);
    }
    cols.subject = b.subject.trim();
  }
  if ("body" in b) {
    if (typeof b.body !== "string" || b.body.length > MAX_BODY) return bad(`body must be a string of at most ${MAX_BODY} characters`);
    cols.body = b.body;
  }
  if ("bodyFormat" in b) {
    if (b.bodyFormat !== "markdown" && b.bodyFormat !== "html") return bad('bodyFormat must be "markdown" or "html"');
    cols.body_format = b.bodyFormat;
  }
  if ("textOverride" in b) {
    if (!str(b.textOverride, true) || (b.textOverride?.length ?? 0) > MAX_BODY) return bad("textOverride must be a string or null");
    cols.text_override = b.textOverride === null || !b.textOverride.trim() ? null : b.textOverride;
  }
  if ("fromEmail" in b) {
    if (!str(b.fromEmail, true) || (b.fromEmail && !isAddress(b.fromEmail))) return bad("fromEmail must be an email address or null");
    cols.from_email = b.fromEmail || null;
  }
  if ("fromName" in b) {
    if (!str(b.fromName, true) || (b.fromName?.length ?? 0) > 200 || /[\r\n]/.test(b.fromName ?? "")) {
      return bad("fromName must be one line of at most 200 characters, or null");
    }
    cols.from_name = b.fromName?.trim() || null;
  }
  if ("segment" in b) {
    const s = parseSegment(b.segment);
    if (typeof s === "string") return bad(s);
    cols.segment = JSON.stringify(s);
  }
  const unknown = unknownVariables(cols.subject ?? null, cols.body ?? null, cols.text_override ?? null);
  if (unknown.length) return bad(`unknown variable(s): ${unknown.map((n) => `{{${n}}}`).join(", ")}`, "unknown_variable");
  return cols;
}

export const isInvalid = (v: Columns | Invalid): v is Invalid => typeof v.error === "string" && "detail" in v;

// senderOf is who a campaign mail comes from: the campaign's own sender, else
// NEWSLETTER_MAIL_FROM / NEWSLETTER_MAIL_FROM_NAME. null = nobody configured.
export function senderOf(c: Campaign, env: Env): { from: string; fromName: string } | null {
  const from = c.fromEmail ?? env.NEWSLETTER_MAIL_FROM ?? "";
  if (!from) return null;
  return { from, fromName: c.fromName ?? env.NEWSLETTER_MAIL_FROM_NAME ?? "" };
}
