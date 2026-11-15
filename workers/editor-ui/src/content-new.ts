// GET {base}/api/content/new-path?section=<folder>&title=<title> — the path a
// new article would get: "<section>/<slug>.md", the slug made from the title
// as ssg makes slugs (slug.ts; transliterated when the site sets
// slug_transliterate), with "-2", "-3"… when the name is taken.
// → {path, slug}. Nothing is written; the panel then publishes the file with
// `version: null` (it must not exist), so two people taking the same name at
// once still meet a conflict, never an overwrite.
//
// Requests to the repository host: the head and the folder's listing.

import { contentFiles } from "./content-list";
import { contentFolder, contentPath } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { OpError } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { fileSlug, freeName } from "./slug";
import { sourceFor } from "./source";

export interface NewPathInput { section?: string | null; title?: string | null }
export interface NewPath { path: string; slug: string }

// newPathIn: the free name in a repository folder (source-git.ts).
export async function newPathIn(s: ContentScope, section: string, title: string | null | undefined): Promise<NewPath> {
  const folder = contentFolder(section, s);
  const { files } = await contentFiles(s, [folder]);
  const slug = fileSlug(title ?? "", s.model.slugTransliterate === true);
  const path = freeName(folder, slug, new Set(files.map((f) => f.path)));
  return { path: contentPath(path, s, "write"), slug };
}

// For an MDDB source, section is "mddb:<collection>" or "mddb:<collection>/<lang>".
export const contentNewPathOp: Op<NewPathInput, NewPath> = {
  name: "content_new_path",
  permission: PERM.read,
  run: async (ctx, input) => {
    if (!input.section) throw new OpError("invalid_input", 400, "section is required");
    return (await sourceFor(ctx, input.section)).newPath(input.section, input.title);
  },
};
