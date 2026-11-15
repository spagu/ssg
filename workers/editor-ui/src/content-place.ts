// Where an article sits in the site: its section (the deepest configured
// section or content folder holding it, with its type) and its language.
//
// The language follows ssg's order (internal/generator/language_sections.go):
// the file's own `lang:` first, then `language_sections` — keyed by the
// file's folder relative to its content folder, longest prefix wins — then
// `default_language`. ssg keys sections relative to the source folder and, for
// a `content_sources` folder, relative to `content_dir`; the editor uses the
// content folder for both, so a key naming a `content_sources` folder by its
// path under `content_dir` is not matched here (the build stays the truth).

import type { SiteModel } from "./site-model";

const under = (path: string, folder: string): boolean => path === folder || path.startsWith(folder + "/");
const deepest = (paths: string[], path: string): string | null =>
  paths.filter((p) => under(path, p)).reduce<string | null>((best, p) => (best === null || p.length > best.length ? p : best), null);

// sectionOf: the deepest section holding `path`, else its content folder.
export function sectionOf(path: string, model: SiteModel): { section: string; type: "page" | "post" | null } {
  const section = deepest(model.sections.map((s) => s.path), path);
  if (section !== null) return { section, type: model.sections.find((s) => s.path === section)!.type };
  // Listed and checked paths are always inside a content folder.
  return { section: deepest(model.contentRoots, path)!, type: null };
}

// languageOf: the article's language, or null when the site says nothing.
export function languageOf(path: string, own: string | null, model: SiteModel): string | null {
  if (own) return own;
  const root = deepest(model.contentRoots, path)!; // as in sectionOf
  const dir = path.slice(0, path.lastIndexOf("/"));
  const rel = dir === root ? "" : dir.slice(root.length + 1);
  let best = "";
  let lang: string | null = null;
  for (const [key, value] of Object.entries(model.languageSections)) {
    const k = key.replace(/^\/+|\/+$/g, ""); // "" never wins: it is not longer than ""
    if (key !== "home" && k.length > best.length && under(rel, k)) {
      best = k;
      lang = value.trim() || null;
    }
  }
  return lang ?? model.defaultLanguage;
}
