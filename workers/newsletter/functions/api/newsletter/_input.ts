// Request parsing and validation for the sign-up endpoint. A leading underscore
// keeps this file out of the Pages route table — it is imported, never served.
//
// Every function here is pure (no bindings, no I/O beyond reading the request
// body), so the rules for what a valid sign-up looks like live in one place.

export type Fields = Record<string, unknown>;

// readBody accepts the two shapes a sign-up arrives in: JSON from the enhanced
// form (newsletter.js) or any fetch caller, and a urlencoded/multipart body from
// a plain HTML form with no script. `isForm` decides the response: a form post
// is answered with a 303 redirect a browser follows, a JSON call with JSON.
export async function readBody(request: Request): Promise<{ fields: Fields; isForm: boolean } | null> {
  const type = (request.headers.get("content-type") || "").toLowerCase();
  try {
    if (type.includes("application/json")) {
      const data = await request.json();
      return data && typeof data === "object" && !Array.isArray(data) ? { fields: data as Fields, isForm: false } : null;
    }
    if (type.includes("application/x-www-form-urlencoded") || type.includes("multipart/form-data")) {
      const form = await request.formData();
      const fields: Fields = {};
      for (const key of new Set(form.keys())) {
        const all = form.getAll(key).map(String);
        // Repeated fields (several tag checkboxes) stay a list; the rest are scalars.
        fields[key] = all.length > 1 ? all : all[0];
      }
      return { fields, isForm: true };
    }
  } catch {
    /* malformed body */
  }
  return null;
}

// text returns a trimmed string capped at max characters, or "".
export function text(raw: unknown, max: number): string {
  return typeof raw === "string" ? raw.trim().slice(0, max) : "";
}

// normaliseEmail lower-cases and validates an address. Deliberately loose (the
// confirmation mail is the real test) but strict about the characters that
// would let one field smuggle a second recipient or a header into a mailer.
export function normaliseEmail(raw: unknown): string | null {
  const email = text(raw, 255).toLowerCase();
  if (email.length > 254) return null;
  if (!/^[^@\s<>()",;:\\]+@[^@\s<>()",;:\\]+\.[^@\s<>()",;:\\]+$/.test(email)) return null;
  if (email.split("@")[0].length > 64) return null;
  return email;
}

// normaliseLang accepts a BCP 47-ish tag ("pl", "pt-BR", "hi") and returns it
// with a lower-case primary subtag, or null.
export function normaliseLang(raw: unknown): string | null {
  const lang = text(raw, 16);
  if (!/^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/.test(lang)) return null;
  const [primary, region] = lang.split("-");
  return region ? `${primary.toLowerCase()}-${region}` : primary.toLowerCase();
}

// normaliseTags takes a list or a comma-separated string and returns at most 10
// lower-case tags of [a-z0-9_-], comma-joined for storage ("" when none).
export function normaliseTags(raw: unknown): string {
  const list = Array.isArray(raw) ? raw : typeof raw === "string" ? raw.split(",") : [];
  const tags = list
    .map((t) => String(t).trim().toLowerCase())
    .filter((t) => /^[a-z0-9_-]{1,32}$/.test(t));
  return [...new Set(tags)].slice(0, 10).join(",");
}

// normalisePath keeps only a same-origin page path (no query, no fragment). A
// leading "//" or any backslash is refused: "/\evil.example" is read by
// browsers as "//evil.example", which would turn the post-sign-up redirect into
// an open redirect.
export function normalisePath(raw: unknown): string | null {
  if (typeof raw !== "string" || !raw.startsWith("/") || raw.startsWith("//") || raw.includes("\\")) return null;
  const clean = raw.split(/[?#]/)[0].slice(0, 512);
  return clean || null;
}

// isConsent reads the consent checkbox / flag. Anything but an explicit yes is
// a no: an unticked HTML checkbox sends nothing at all.
export function isConsent(raw: unknown): boolean {
  if (raw === true) return true;
  return typeof raw === "string" && ["1", "true", "yes", "on"].includes(raw.trim().toLowerCase());
}

// thanksURL resolves where a plain form post is redirected. NEWSLETTER_THANKS_URL
// is either a URL with an optional {lang} placeholder ("/{lang}/thanks/") or a
// JSON map {"en": "/thanks/", "pl": "/pl/dziekujemy/", "default": "/thanks/"}.
// Unset, the visitor goes back to the page they signed up on, at the
// #newsletter-thanks fragment the stock form uses to show its message without JS.
export function thanksURL(setting: string | undefined, lang: string | null, sourcePage: string | null): string {
  const raw = (setting || "").trim();
  const primary = lang ? lang.split("-")[0] : null;
  if (raw.startsWith("{")) {
    try {
      const map = JSON.parse(raw) as Record<string, string>;
      const pick = (lang && map[lang]) || (primary && map[primary]) || map.default || map.en;
      if (typeof pick === "string" && pick) return pick;
    } catch {
      /* a malformed map falls through to the default below */
    }
  } else if (raw) {
    return raw.replaceAll("{lang}", primary || "en");
  }
  return `${sourcePage || "/"}#newsletter-thanks`;
}
