// Strings in five languages (en, pl, hi, de, fr — the `newsletter` template's
// set), overridable without editing the panel, the way newsletter.js does it:
//
//   <script id="newsletter-admin-config" type="application/json">
//     {"lang": "pl", "i18n": {"pl": {"nav.dashboard": "Start"}}}
//   </script>
//
// A JSON script block is data, not script: the browser never executes a
// non-JavaScript type, so it needs no CSP exception (script-src governs only
// executable scripts). The panel reads it with JSON.parse.
//
// A value is a string with {name} placeholders, or an object of plural forms
// ({one, few, many, other}) chosen with Intl.PluralRules for vars.count.

export const LANGS = ["en", "pl", "hi", "de", "fr"];

// pickLang returns the first supported language among the candidates
// (<html lang>, then navigator.languages), compared by primary subtag.
export function pickLang(candidates) {
  for (const c of candidates) {
    const primary = String(c || "").toLowerCase().split(/[-_]/)[0];
    if (LANGS.includes(primary)) return primary;
  }
  return "en";
}

// readConfig parses the JSON config block's text; anything unreadable is {}.
export function readConfig(textContent) {
  try {
    const v = JSON.parse(textContent || "{}");
    return v && typeof v === "object" && !Array.isArray(v) ? v : {};
  } catch {
    return {};
  }
}

const fill = (s, vars) => s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));

// createI18n: `tables` = {en: {...}, <lang>: {...}}; overrides = config.i18n.
// Lookup order: override for lang, lang, override for en, en, the key itself
// (a missing string shows its key instead of nothing, and a test catches it).
export function createI18n(lang, tables, overrides = {}, format = (n) => String(n)) {
  const chain = [overrides[lang], tables[lang], overrides.en, tables.en].filter(Boolean);
  const rules = new Intl.PluralRules(lang);
  const lookup = (key) => {
    for (const table of chain) if (Object.hasOwn(table, key)) return table[key];
    return undefined;
  };
  function t(key, vars = {}) {
    let v = lookup(key);
    if (v === undefined) return key;
    if (typeof v === "object") v = v[rules.select(Number(vars.count))] ?? v.other;
    const shown = "count" in vars ? { ...vars, count: format(vars.count) } : vars;
    return fill(String(v), shown);
  }
  return { lang, t, has: (key) => lookup(key) !== undefined };
}
