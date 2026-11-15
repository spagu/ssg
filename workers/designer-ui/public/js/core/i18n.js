// Strings in five languages (en, pl, hi, de, fr — the set every ssg panel
// speaks), overridable without editing the panel through a JSON block in
// index.html, which is data, never executed (so the CSP needs no exception):
//
//   <script id="panel-config" type="application/json">
//     {"lang": "pl", "i18n": {"pl": {"nav.options": "Opcje"}}}
//   </script>
//
// A value is a string with {name} placeholders, or an object of plural forms
// ({one, few, many, other}) chosen with Intl.PluralRules for vars.count.
// Generic — no panel-specific import, so another panel can copy it as is.

export const LANGS = ["en", "pl", "hi", "de", "fr"];

// pickLang returns the first supported language among the candidates,
// compared by primary subtag ("pl-PL" → "pl"); "en" when none is.
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
