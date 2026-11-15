// The repository as a content source: what the content operations did before
// sources existed (GO-170, GO-172), unchanged — each call opens the content
// scope (repository, branch, site model at the head) first, exactly as the
// operation did, then the same function does the same requests.

import { publish } from "./change-publish";
import { listContent } from "./content-list";
import { newPathIn } from "./content-new";
import { readArticle } from "./content-read";
import { contentScope } from "./content-scope";
import type { OpContext } from "./ops";
import { diffRevisions } from "./revision-diff";
import { readRevision } from "./revision-read";
import { listRevisions } from "./revisions";
import type { ContentSource } from "./source";

export function gitSource(ctx: OpContext): ContentSource {
  const scope = () => contentScope(ctx);
  return {
    origin: "git",
    list: async (input) => listContent(await scope(), input),
    read: async (path) => readArticle(await scope(), path),
    newPath: async (section, title) => newPathIn(await scope(), section, title),
    publish: (body) => publish(ctx, body),
    history: async (path, cursor) => listRevisions(await scope(), path, cursor),
    readAt: async (path, id, cursor) => readRevision(await scope(), path, id, cursor),
    diff: async (input) => diffRevisions(await scope(), input),
  };
}
