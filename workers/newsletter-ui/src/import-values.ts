// Cell values of an uploaded list, read the way mailing tools write them: a
// status word from any of the usual vocabularies, a date in any of the usual
// formats, a sign-up source that may be a whole URL.

// What a row says about the person. `active` becomes confirmed or pending by
// the admin's choice; the others decide what the import does with the row.
export type FileStatus = "active" | "pending" | "unsubscribed" | "bounced" | "complaint";

// Status words, compared lower-case with _ - . and runs of spaces as one space.
const VOCABULARY: Record<FileStatus, readonly string[]> = {
  active: ["subscribed", "subscriber", "active", "confirmed", "enabled", "opted in", "opt in", "optin", "double opt in", "verified"],
  pending: ["pending", "unconfirmed", "not confirmed", "awaiting confirmation", "unverified"],
  unsubscribed: ["unsubscribed", "unsubscribe", "opted out", "opt out", "optout", "removed", "inactive", "disabled", "deleted", "archived"],
  bounced: ["bounced", "bounce", "cleaned", "invalid", "hard bounce", "hard bounced", "hardbounce", "undeliverable"],
  complaint: ["complained", "complaint", "spam", "spam complaint", "marked as spam", "abuse"],
};
const WORDS = new Map(Object.entries(VOCABULARY).flatMap(([status, words]) => words.map((w) => [w, status as FileStatus])));

export const normaliseStatus = (raw: string): FileStatus | null =>
  WORDS.get(raw.toLowerCase().replace(/[\s_.-]+/g, " ").trim()) ?? null;

// isSet tells whether a date cell says "this happened". Some exports fill an
// unused date column with a placeholder rather than leaving it empty; those
// must not mark the person unsubscribed or bounced.
export const isSet = (raw: string): boolean => raw !== "" && !/^(0|false|no|null|none|never|n\/a|-)$/i.test(raw);

export type DateOrder = "dmy" | "mdy";
export const isDateOrder = (raw: unknown): raw is DateOrder => raw === "dmy" || raw === "mdy";

const TIME = String.raw`(?:[ T,]+(\d{1,2}):(\d{2})(?::(\d{2})(?:[.,]\d+)?)?)?`;
const ZONE = String.raw`\s*(Z|UTC|GMT|[+-]\d{2}:?\d{2})?`;
const ISO = new RegExp(String.raw`^(\d{4})-(\d{1,2})-(\d{1,2})${TIME}${ZONE}$`, "i");
const YMD_SLASH = new RegExp(String.raw`^(\d{4})/(\d{1,2})/(\d{1,2})${TIME}${ZONE}$`, "i");
const DOTTED = new RegExp(String.raw`^(\d{1,2})\.(\d{1,2})\.(\d{4})${TIME}${ZONE}$`, "i");
const SLASH = new RegExp(String.raw`^(\d{1,2})/(\d{1,2})/(\d{4})${TIME}${ZONE}$`, "i");
const UNIX = /^(\d{9,10}|\d{12,13})$/; // seconds or milliseconds since 1970

// inferDateOrder reads a whole column of day/month/year-with-slashes dates.
// A first part above 12 can only be a day, a second part above 12 only a day
// in month-first order; one such value decides for the column. A column that
// never decides (every value like 03/04/2024) returns null: guessing would
// silently swap days and months for half the year. 05/05/2024 reads the same
// either way and needs no decision.
export function inferDateOrder(values: string[]): DateOrder | null {
  let dmy = false;
  let mdy = false;
  let open = false;
  for (const v of values) {
    const m = SLASH.exec(v);
    if (!m) continue;
    const [a, b] = [Number(m[1]), Number(m[2])];
    if (a > 12) dmy = true;
    else if (b > 12) mdy = true;
    else if (a !== b) open = true;
  }
  if (dmy !== mdy) return dmy ? "dmy" : "mdy";
  return dmy || open ? null : "dmy"; // both seen = contradictory; neither = nothing to decide
}

// at builds the instant, refusing impossible dates (31.02.2024) and times.
function at(y: string, mo: string, d: string, h = "0", mi = "0", s = "0", zone = ""): number | null {
  const [Y, M, D, H, I, S] = [y, mo, d, h, mi, s].map(Number);
  if (M < 1 || M > 12 || D < 1 || H > 23 || I > 59 || S > 59) return null;
  const ms = Date.UTC(Y, M - 1, D, H, I, S);
  if (new Date(ms).getUTCDate() !== D) return null;
  const z = /^([+-])(\d{2}):?(\d{2})$/.exec(zone);
  return z ? ms - (z[1] === "-" ? -1 : 1) * (Number(z[2]) * 60 + Number(z[3])) * 60_000 : ms;
}

function instant(raw: string, order: DateOrder): number | null {
  let m: RegExpExecArray | null;
  if (UNIX.test(raw)) return raw.length <= 10 ? Number(raw) * 1000 : Number(raw);
  if ((m = ISO.exec(raw) ?? YMD_SLASH.exec(raw))) return at(m[1], m[2], m[3], m[4], m[5], m[6], m[7]);
  if ((m = DOTTED.exec(raw))) return at(m[3], m[2], m[1], m[4], m[5], m[6], m[7]);
  if ((m = SLASH.exec(raw))) return order === "dmy" ? at(m[3], m[2], m[1], m[4], m[5], m[6], m[7]) : at(m[3], m[1], m[2], m[4], m[5], m[6], m[7]);
  return null;
}

// parseDate reads one cell. A time without a zone is taken as UTC. A date in
// the future is refused like an unreadable one: it would sort the person
// ahead of every real sign-up and claim consent that has not happened yet.
export function parseDate(raw: string, order: DateOrder, now: number): { iso: string } | { warning: "unparseable" | "future" } {
  const ms = instant(raw, order);
  if (ms === null) return { warning: "unparseable" };
  return ms > now ? { warning: "future" } : { iso: new Date(ms).toISOString() };
}

// sourcePath keeps what the `newsletter` template stores in source_page: a
// same-origin page path (its normalisePath rule). A full URL keeps its path;
// anything else — free text such as "landing form" — is not stored, so the
// column keeps one shape for every row.
export function sourcePath(raw: string): string | null {
  let path = raw;
  if (/^https?:\/\//i.test(raw)) {
    try {
      path = new URL(raw).pathname;
    } catch {
      return null;
    }
  }
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\")) return null;
  return path.split(/[?#]/)[0].slice(0, 512);
}
