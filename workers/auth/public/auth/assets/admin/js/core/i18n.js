// Strings in five languages (en, pl, hi, de, fr — the worker's pages' set),
// the newsletter panel's approach: a value is a string with {name}
// placeholders, or an object of plural forms ({one, few, many, other})
// chosen with Intl.PluralRules for vars.count. A missing string shows its key
// (and the strings test catches it before that can ship).

export const LANGS = ["en", "pl", "hi", "de", "fr"];

// pickLang returns the first supported language among the candidates,
// compared by primary subtag; English otherwise.
export function pickLang(candidates) {
  for (const c of candidates) {
    const primary = String(c || "").toLowerCase().split(/[-_]/)[0];
    if (LANGS.includes(primary)) return primary;
  }
  return "en";
}

// readConfig parses the shell's JSON data block; anything unreadable is {}.
export function readConfig(textContent) {
  try {
    const v = JSON.parse(textContent || "{}");
    return v && typeof v === "object" && !Array.isArray(v) ? v : {};
  } catch {
    return {};
  }
}

const fill = (s, vars) => s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));

// createI18n: tables = {en: {...}, <lang>: {...}}; lookup: lang, then en, then the key.
export function createI18n(lang, tables, format = (n) => String(n)) {
  const chain = [tables[lang], tables.en].filter(Boolean);
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
