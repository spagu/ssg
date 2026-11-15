// The article list (GET api/content) as the panel shows it: newest first,
// filtered by language and state on the page (the worker filters by section
// and by words in the path itself), with a mark on the articles that have an
// unpublished draft in this browser. Pure.

const dateKey = (item) => (typeof item.date === "string" ? item.date : "");

// fileName: the last part of a path without ".md" (also an MDDB key).
export const fileName = (path) => String(path).split("/").pop().replace(/\.md$/, "");
export const titleOfItem = (item) => (item.title && String(item.title).trim()) || fileName(item.path);

// byNewest: dated articles newest first, then undated ones, then by title.
export function byNewest(a, b) {
  const d = dateKey(b).localeCompare(dateKey(a));
  return d || titleOfItem(a).localeCompare(titleOfItem(b)) || a.path.localeCompare(b.path);
}

// STATES: the "Show" filter. local = a draft in this browser.
export const STATES = ["all", "published", "drafts", "local"];

// filterItems: the items to show for {language, state}; `local` = Set of
// paths with a draft here.
export function filterItems(items, { language = "", state = "all" } = {}, local = new Set()) {
  return items
    .filter((i) => !language || i.language === language)
    .filter((i) => state === "all" || (state === "drafts" ? i.draft === true : state === "published" ? i.draft !== true : local.has(i.path)))
    .slice()
    .sort(byNewest);
}

// languagesOf: the site's languages, then any an article names besides.
export function languagesOf(items, site = []) {
  const out = [...site];
  for (const i of items) if (i.language && !out.includes(i.language)) out.push(i.language);
  return out;
}

// sectionsOf: the site's sections (api/site), then any listed article's
// section not among them (a deeper folder the configuration does not name).
export function sectionsOf(items, site = []) {
  const out = site.map((s) => s.path);
  for (const i of items) if (i.section && !out.includes(i.section)) out.push(i.section);
  return out;
}

// sectionLabel: how a section reads in a menu: an MDDB collection by name.
export const sectionLabel = (path) => (String(path).startsWith("mddb:") ? `${path.slice(5)} (MDDB)` : path);

// newDrafts: the stored drafts of articles not published yet, newest first.
export const newDrafts = (drafts) => drafts.filter((d) => d.isNew).slice().sort((a, b) => String(b.savedAt).localeCompare(String(a.savedAt)));
