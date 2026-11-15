// The file name of a new article, from its title — the way ssg makes a slug
// (internal/models/content.go, Slugify), so the name the editor proposes is
// the URL segment ssg would derive from the same title. By default ssg does
// not transliterate: a letter outside a–z and 0–9 after Go's lower-casing
// becomes a hyphen ("Zażółć gęślą" → "za-g-l"). With `slug_transliterate: true`
// (#426) it folds Latin diacritics first ("zazolc-gesla"): slugifyTranslit,
// below. Reproducing both keeps the two from ever disagreeing about a URL.
//
// Go lower-cases rune by rune (unicode.ToLower). Only three kinds of rune end
// up as a–z or 0–9 there: A–Z, U+0130 (İ → i) and U+212A (Kelvin sign → k).
// JavaScript's toLowerCase differs exactly there ("İ" → "i̇", two code
// points), so the table is written out instead. test/slug.test.ts checks it
// against a sample generated from the Go function, every rune included.

// GO_LOWER_TO_ASCII: the non-ASCII runes Go lower-cases to an ASCII letter.
const GO_LOWER_TO_ASCII: Record<string, string> = { "\u0130": "i", "\u212A": "k" };

// slugify is ssg's models.Slugify: lower case a–z and 0–9, every run of
// anything else collapsed to one hyphen, hyphens trimmed at both ends.
export function slugify(s: string): string {
  let out = "";
  let dash = false;
  for (const ch of s) {
    const c = /^[A-Z]$/.test(ch) ? ch.toLowerCase() : (GO_LOWER_TO_ASCII[ch] ?? ch);
    if (/^[a-z0-9]$/.test(c)) {
      out += c;
      dash = false;
    } else if (!dash) {
      out += "-";
      dash = true;
    }
  }
  return out.replace(/^-+|-+$/g, "");
}

// LATIN_FOLD: ssg's latinFold (internal/models/slug_translit.go) — the Latin
// letters NFD leaves whole, and the ASCII written for them.
const LATIN_FOLD: Record<string, string> = {
  "\u00df": "ss", "\u00e6": "ae", "\u0153": "oe", "\u00f8": "o", "\u0142": "l", "\u0111": "d", "\u00f0": "d",
  "\u00fe": "th", "\u0131": "i", "\u0127": "h", "\u0140": "l", "\u0133": "ij", "\u0167": "t",
};

// slugifyTranslit is ssg's models.SlugifyTranslit: lower case, NFD, combining
// marks (Mn) dropped, LATIN_FOLD applied, then slugify. Other scripts are not
// romanised, so a title in Hindi still slugs to "". JavaScript's full
// lower-casing differs from Go's simple one only where NFD and slugify erase
// the difference (İ → i̇ → i); test/slug.test.ts checks every code point.
export function slugifyTranslit(s: string): string {
  const folded = s.toLowerCase().normalize("NFD").replace(/\p{Mn}/gu, "");
  return slugify(Array.from(folded, (ch) => LATIN_FOLD[ch] ?? ch).join(""));
}

// FALLBACK: the name of an article whose title has no letter ssg keeps (a
// title in Hindi or Japanese slugs to ""), so a new file still gets a name.
export const FALLBACK = "untitled";

// MAX_SLUG: a slug longer than this is cut at a hyphen — a file name stays
// readable and well inside the path policy's limits.
export const MAX_SLUG = 80;

// fileSlug: the slug used for a new file's name (never empty, at most
// MAX_SLUG); translit follows the site's slug_transliterate.
export function fileSlug(title: string, translit = false): string {
  let s = translit ? slugifyTranslit(title) : slugify(title);
  if (s.length > MAX_SLUG) {
    s = s.slice(0, MAX_SLUG);
    const cut = s.lastIndexOf("-");
    s = cut > 0 ? s.slice(0, cut) : s;
  }
  return s || FALLBACK;
}

// freeName: "<slug>.md" in `folder`, or "<slug>-2.md", "<slug>-3.md"… — the
// first name not in `taken` (repository paths).
export function freeName(folder: string, slug: string, taken: ReadonlySet<string>): string {
  let path = `${folder}/${slug}.md`;
  for (let n = 2; taken.has(path); n++) path = `${folder}/${slug}-${n}.md`;
  return path;
}
