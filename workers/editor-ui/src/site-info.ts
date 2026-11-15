// GET {base}/api/site — what the panel needs to know about the site before it
// lists or creates anything (GO-173): the folders a new article can go in,
// the languages, the shortcode names (the preview labels their calls), and
// where pasted images go with the publication limits. Everything comes from
// the site model the other operations already read (site-model.ts); nothing
// here decides a permission or a path — the operations still check both.
//
//   → {sections: [{path, type, origin}], languages, defaultLanguage,
//      shortcodes: [name], media: {dir, urlBase} | null, limits}
//
// `sections`: the configured sections, then the content folders not already
// listed (an article may sit loose in one), then — when MDDB is configured —
// "mddb:<collection>" for each collection the site reads. A site with MDDB
// only (no repository) answers its MDDB sections and nothing else. `media`
// is null when there is no folder ssg publishes images from (image ops then
// answer 503 media_not_configured; the panel turns pasting off).
//
// Requests to the repository host: those of the site model (the head, and the
// configuration when its cache is cold).

import { contentScope } from "./content-scope";
import { LIMITS } from "./media-change";
import { mediaPlace } from "./media-scope";
import { mddbConfigFrom, mddbConfigured } from "./mddb-config";
import { MDDB_PREFIX } from "./mddb-path";
import type { Op } from "./ops";
import { PERM } from "./permissions";
import type { ChangeLimits } from "../portable/repo-provider";
import { repositoryOf } from "./repo";
import type { SiteModel } from "./site-model";

export interface SiteSection { path: string; type: "page" | "post" | null; origin: "git" | "mddb" }
export interface SiteInfo {
  sections: SiteSection[];
  languages: string[];
  defaultLanguage: string | null;
  shortcodes: string[];
  media: { dir: string; urlBase: string } | null;
  limits: ChangeLimits;
}

// gitSections: configured sections first, then loose content folders.
function gitSections(model: SiteModel): SiteSection[] {
  const out: SiteSection[] = model.sections.map((s) => ({ path: s.path, type: s.type, origin: "git" }));
  for (const root of model.contentRoots) if (!out.some((s) => s.path === root)) out.push({ path: root, type: null, origin: "git" });
  return out;
}

export const siteInfoOp: Op<void, SiteInfo> = {
  name: "site_info",
  permission: PERM.read,
  run: async (ctx) => {
    // Without a repository and without MDDB, contentScope answers the 503
    // that names what to set.
    const model = repositoryOf(ctx.env, ctx.deps).ok || !mddbConfigured(ctx.env) ? (await contentScope(ctx)).model : null;
    const sections = model ? gitSections(model) : [];
    if (mddbConfigured(ctx.env)) {
      const c = mddbConfigFrom(ctx.env, model);
      const seen = new Set<string>();
      for (const s of c.ok ? c.config.sources : []) {
        if (seen.has(s.collection)) continue;
        seen.add(s.collection);
        sections.push({ path: MDDB_PREFIX + s.collection, type: s.type, origin: "mddb" });
      }
    }
    const place = model ? mediaPlace(model) : null;
    return {
      sections,
      languages: model?.languages ?? [],
      defaultLanguage: model?.defaultLanguage ?? null,
      shortcodes: model ? model.shortcodes.map((s) => s.name) : [],
      media: place ? { dir: place.dir, urlBase: place.urlBase } : null,
      limits: LIMITS,
    };
  },
};
