// Names and descriptions of settings in the panel's language. The schema's
// English description is always there; the panel's strings add a label for
// every group and, for the most used plain keys (TRANSLATED), a label and a
// description in all five languages. A key without them shows its name as
// words and the English description, marked lang="en" — never a blank.
//
//   group.<id>.title / group.<id>.desc     every group, ADVANCED included
//   key.<path>      / desc.<path>          the keys listed in TRANSLATED

import { ADVANCED, humanise } from "./form-model.js";

// TRANSLATED: the plain keys an owner reaches for first (analytics_ids was
// removed in GO-183: it loads a vendor script on every page, so it is "executes";
// domain and redirects left with SR-10: they can send visitors elsewhere).
export const TRANSLATED = [
  "title", "description", "base_path", "colors", "template", "attribution",
  "marketing.og_image", "marketing.og_site_name", "marketing.twitter_site",
  "languages", "default_language", "timezone",
  "paginate", "home_posts_limit", "auto_excerpt", "post_url_format", "pretty_urls",
  "toc", "toc_depth", "highlight", "math", "mermaid", "story.enabled",
  "feed", "feed_items", "feed_full_content",
  "seo", "sitemap_off", "robots_off", "search_index",
  "webp", "webp_quality", "minify_all",
];

const pathOf = (entry) => entry.segments.join(".");

// labelOf: the key's label — translated, else its last key as words.
export function labelOf(i18n, entry) {
  const k = `key.${pathOf(entry)}`;
  return i18n.has(k) ? i18n.t(k) : humanise(entry.segments[entry.segments.length - 1]);
}

// descriptionOf: {text, lang} — the translation, else the schema's English
// (lang "en", so a screen reader reads it as English), else null.
export function descriptionOf(i18n, entry) {
  const k = `desc.${pathOf(entry)}`;
  if (i18n.lang !== "en" && i18n.has(k)) return { text: i18n.t(k), lang: i18n.lang };
  const en = entry.node.description;
  return en ? { text: en[0].toUpperCase() + en.slice(1), lang: "en" } : null;
}

// groupText: a group's title and description in the panel's language, the
// schema's English when the strings lack it.
export function groupText(i18n, group) {
  const pick = (part, fallback) => {
    const k = `group.${group.id}.${part}`;
    return i18n.has(k) ? i18n.t(k) : fallback;
  };
  const advancedTitle = group.id === ADVANCED ? "Advanced" : humanise(group.id);
  return { title: pick("title", group.title || advancedTitle), description: pick("desc", group.description) };
}

// searchTexts: what the search looks in for an entry — both languages.
export function searchTexts(i18n, entry) {
  const d = descriptionOf(i18n, entry);
  return [labelOf(i18n, entry), humanise(entry.segments[entry.segments.length - 1]), d?.text ?? "", entry.node.description ?? ""];
}
