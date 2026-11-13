// Who a campaign goes to. There are no separate lists: a segment is always
// "confirmed subscribers", narrowed by language and/or tags. Stored as JSON in
// campaigns.segment; segmentWhere is the single place that turns it into SQL,
// for the preview count here and for the send engine's batches later.
//
// Tags live in the `newsletter` template's subscribers.tags column as a
// comma-joined list of lower-case [a-z0-9_-] words ("news,offers"). A tag
// matches only as a whole word: "new" never matches "news". Languages match by
// value, and a bare language also matches its regional forms ("pt" matches
// "pt-BR"; "pt-BR" matches only "pt-BR").

import type { Env } from "./lib";

export interface Segment {
  languages: string[];
  tags: string[];
  tagMode: "any" | "all";
}

export const EMPTY_SEGMENT: Segment = { languages: [], tags: [], tagMode: "any" };

const MAX_ITEMS = 50;
const TAG = /^[a-z0-9_-]{1,64}$/;
const LANG = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

// The `newsletter` template's spelling: lower-case language, upper-case region.
const normaliseLang = (l: string): string => {
  const [primary, region] = l.split("-");
  return region ? `${primary.toLowerCase()}-${region.toUpperCase()}` : primary.toLowerCase();
};

// parseSegment validates untrusted input (the API body, or a stored value) and
// returns a normalised segment, or the problem as a string.
export function parseSegment(raw: unknown): Segment | string {
  if (raw === undefined || raw === null) return { ...EMPTY_SEGMENT };
  if (typeof raw !== "object" || Array.isArray(raw)) return "segment must be an object";
  const r = raw as Record<string, unknown>;
  const list = (key: string): string[] | string => {
    const v = r[key] ?? [];
    if (!Array.isArray(v) || v.length > MAX_ITEMS || !v.every((x) => typeof x === "string")) {
      return `segment.${key} must be a list of at most ${MAX_ITEMS} strings`;
    }
    return v as string[];
  };
  const languages = list("languages");
  if (typeof languages === "string") return languages;
  const tags = list("tags");
  if (typeof tags === "string") return tags;
  const badLang = languages.find((l) => !LANG.test(l));
  if (badLang !== undefined) return `segment.languages: ${JSON.stringify(badLang)} is not a language code`;
  const lowered = tags.map((t) => t.trim().toLowerCase());
  const badTag = lowered.find((t) => !TAG.test(t));
  if (badTag !== undefined) return `segment.tags: ${JSON.stringify(badTag)} is not a tag (a-z, 0-9, _ and -)`;
  const tagMode = r.tagMode ?? "any";
  if (tagMode !== "any" && tagMode !== "all") return 'segment.tagMode must be "any" or "all"';
  return { languages: [...new Set(languages.map(normaliseLang))], tags: [...new Set(lowered)], tagMode };
}

// segmentWhere is the WHERE clause (without "WHERE") for the subscribers table.
// Values are always bound, never spliced.
export function segmentWhere(segment: Segment): { sql: string; binds: string[] } {
  const parts = ["status = 'confirmed'"];
  const binds: string[] = [];
  if (segment.languages.length) {
    const alts = segment.languages.map((l) => {
      if (l.includes("-")) {
        binds.push(l);
        return "language = ?";
      }
      binds.push(l, `${l}-%`);
      return "(language = ? OR language LIKE ?)";
    });
    parts.push(`(${alts.join(" OR ")})`);
  }
  if (segment.tags.length) {
    // instr, not LIKE: a tag may contain "_", which LIKE reads as a wildcard.
    const one = "instr(',' || tags || ',', ?) > 0";
    for (const t of segment.tags) binds.push(`,${t},`);
    parts.push(`(${segment.tags.map(() => one).join(segment.tagMode === "all" ? " AND " : " OR ")})`);
  }
  return { sql: parts.join(" AND "), binds };
}

export interface SegmentCount {
  total: number;
  byLanguage: Record<string, number>; // "" = no language stored
}

// countSegment answers "how many would get it", in total and per language.
export async function countSegment(env: Env, segment: Segment): Promise<SegmentCount> {
  const { sql, binds } = segmentWhere(segment);
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT COALESCE(language, '') AS language, COUNT(*) AS n FROM subscribers WHERE ${sql} GROUP BY COALESCE(language, '') ORDER BY language`,
  )
    .bind(...binds)
    .all<{ language: string; n: number }>();
  const byLanguage: Record<string, number> = {};
  let total = 0;
  for (const r of results) {
    byLanguage[r.language] = r.n;
    total += r.n;
  }
  return { total, byLanguage };
}
