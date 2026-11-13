// Input rules for the admin API. They are a copy of the `newsletter` template's
// functions/api/newsletter/_input.ts (the sign-up form): an address, a language
// or a tag the admin enters must be one the form would have accepted, so
// whatever is in the table always has one shape. The templates are scaffolded
// independently, which is why this is a copy and not an import — keep the
// regular expressions in step with that file.
//
// One deliberate difference: the form silently drops an invalid tag (a visitor
// cannot fix it), the admin API refuses it (an admin can).

export const STATUSES = ["pending", "confirmed", "unsubscribed"] as const;
export type Status = (typeof STATUSES)[number];

export const isStatus = (raw: unknown): raw is Status => typeof raw === "string" && (STATUSES as readonly string[]).includes(raw);

// text returns a trimmed string capped at max characters, or "".
export function text(raw: unknown, max: number): string {
  return typeof raw === "string" ? raw.trim().slice(0, max) : "";
}

// normaliseEmail lower-cases and validates an address, exactly as the sign-up
// form does: loose, but strict about characters that could smuggle a second
// recipient or a header into a mailer.
export function normaliseEmail(raw: unknown): string | null {
  const email = text(raw, 255).toLowerCase();
  if (email.length > 254) return null;
  if (!/^[^@\s<>()",;:\\]+@[^@\s<>()",;:\\]+\.[^@\s<>()",;:\\]+$/.test(email)) return null;
  if (email.split("@")[0].length > 64) return null;
  return email;
}

// normaliseLang accepts a BCP 47-ish tag ("pl", "pt-BR") and returns it with a
// lower-case primary subtag, or null.
export function normaliseLang(raw: unknown): string | null {
  const lang = text(raw, 16);
  if (!/^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/.test(lang)) return null;
  const [primary, region] = lang.split("-");
  return region ? `${primary.toLowerCase()}-${region}` : primary.toLowerCase();
}

export const TAG_RE = /^[a-z0-9_-]{1,32}$/;
export const MAX_TAGS = 10;

// parseTags takes a list or a comma-separated string and returns the
// de-duplicated lower-case tags, or null when any entry is not a valid tag or
// there are more than MAX_TAGS. Empty entries ("a,,b", a trailing comma) are
// ignored, as the form ignores them.
export function parseTags(raw: unknown): string[] | null {
  if (raw === null || raw === undefined) return [];
  const list = Array.isArray(raw) ? raw : typeof raw === "string" ? raw.split(",") : null;
  if (!list) return null;
  const tags: string[] = [];
  for (const entry of list) {
    if (typeof entry !== "string") return null;
    const t = entry.trim().toLowerCase();
    if (!t) continue;
    if (!TAG_RE.test(t)) return null;
    if (!tags.includes(t)) tags.push(t);
  }
  return tags.length <= MAX_TAGS ? tags : null;
}

// splitTags turns the stored comma-joined column back into a list.
export const splitTags = (stored: unknown): string[] => (typeof stored === "string" && stored ? stored.split(",") : []);

// escapeLike makes a user string literal inside `LIKE ? ESCAPE '\'`: without it
// a search for "a_b" would also match "axb", and "%" would match everything.
export const escapeLike = (s: string): string => s.replace(/[\\%_]/g, (c) => "\\" + c);

// readJSONObject parses a JSON object body, or returns null.
export async function readJSONObject(request: Request): Promise<Record<string, unknown> | null> {
  try {
    const data = await request.json();
    return data && typeof data === "object" && !Array.isArray(data) ? (data as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

// isTrue reads a boolean flag from JSON (true) or a query string ("1", "true").
export const isTrue = (raw: unknown): boolean => raw === true || (typeof raw === "string" && ["1", "true", "yes"].includes(raw.trim().toLowerCase()));
