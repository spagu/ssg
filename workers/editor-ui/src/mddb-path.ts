// How an MDDB article is named in the editor: one string in the place of a
// repository path, so every content operation keeps its input and output
// shapes (`path`, `version`) whichever source holds the article:
//
//   mddb:<collection>/<lang>/<key>        e.g. mddb:blog/en_US/hello-world
//
// MDDB addresses a document by collection, key and language (docs/API.md,
// POST /v1/get); the key may itself hold "/" (URL-shaped keys), so it is the
// rest of the string after the language. The "mddb:" prefix cannot start a
// path the repository policy accepts as content, so the two never meet.

export const MDDB_PREFIX = "mddb:";

export interface MddbRef { collection: string; lang: string; key: string }

// COLLECTION / LANG: the shapes the editor accepts (MDDB itself takes more;
// a site's collection and language codes are plain identifiers).
const COLLECTION = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const LANG = /^[A-Za-z]{2,3}(?:[_-][A-Za-z0-9]{2,8})*$/;
// MAX_KEY: bytes of a key, as the path policy bounds a repository path.
export const MAX_KEY = 400;

export const isMddbPath = (raw: unknown): raw is string => typeof raw === "string" && raw.startsWith(MDDB_PREFIX);
export const validCollection = (s: string): boolean => COLLECTION.test(s);
export const validLang = (s: string): boolean => LANG.test(s);

// keyProblem: why a key is not one the editor writes, or null. No control
// characters, no surrounding space, no empty or dot-only segment — the key
// becomes the page's slug (internal/mddb/converter.go), so it must read as a
// path segment does.
export function keyProblem(key: string): string | null {
  if (key === "" || new TextEncoder().encode(key).length > MAX_KEY) return "the key is empty or too long";
  if (/[\p{Cc}\p{Cf}\p{Cs}]/u.test(key) || key !== key.trim()) return "the key holds control characters or surrounding space";
  if (key.split("/").some((s) => s === "" || s === "." || s === "..")) return "the key has an empty, \".\" or \"..\" segment";
  return null;
}

// parseMddbPath: the reference, or the reason the string is not one.
export function parseMddbPath(raw: string): { ok: true; ref: MddbRef } | { ok: false; reason: string } {
  const rest = raw.slice(MDDB_PREFIX.length);
  const a = rest.indexOf("/");
  const b = a < 0 ? -1 : rest.indexOf("/", a + 1);
  if (b < 0) return { ok: false, reason: "an MDDB article is mddb:<collection>/<lang>/<key>" };
  const ref = { collection: rest.slice(0, a), lang: rest.slice(a + 1, b), key: rest.slice(b + 1) };
  if (!validCollection(ref.collection)) return { ok: false, reason: "not a collection name" };
  if (!validLang(ref.lang)) return { ok: false, reason: "not a language code" };
  const problem = keyProblem(ref.key);
  return problem ? { ok: false, reason: problem } : { ok: true, ref };
}

export const mddbPath = (r: MddbRef): string => `${MDDB_PREFIX}${r.collection}/${r.lang}/${r.key}`;
