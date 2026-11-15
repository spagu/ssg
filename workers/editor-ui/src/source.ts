// Content sources (GO-175): where an article lives — a file in the site's
// repository (source-git.ts) or a document in MDDB (source-mddb.ts). The
// content operations (content_list, content_read, content_new_path,
// change_publish, revisions_list, revision_read, revision_diff) take the same
// input and give the same answer whichever source serves them; the path says
// which: "mddb:<collection>/<lang>/<key>" (mddb-path.ts) is MDDB, anything
// else the repository. Without EDITOR_MDDB_URL every path goes to the
// repository, so a site without MDDB sees exactly what it saw before.
//
// The operations call these functions from their run(), and source-git.ts
// calls the operations' own git functions back: an import cycle on purpose,
// harmless because every use is inside a function, never at module load.

import { parseRequest } from "./change-input";
import type { ChangeRequest } from "./change-input";
import type { Published } from "./change-publish";
import { decodeCursor, encodeCursor, limitOf, listContent } from "./content-list";
import type { ListInput, ListPage } from "./content-list";
import type { NewPath } from "./content-new";
import type { Article } from "./content-read";
import { contentScope } from "./content-scope";
import { RepoError } from "../portable/repo-provider";
import { mddbConfigFrom, mddbConfigured } from "./mddb-config";
import { MDDB_PREFIX, isMddbPath } from "./mddb-path";
import { OpError } from "./ops";
import type { OpContext } from "./ops";
import { SUBREQUESTS, repositoryOf } from "./repo";
import type { DiffInput, RevisionDiff } from "./revision-diff";
import type { RevisionText } from "./revision-read";
import type { RevisionPage } from "./revisions";
import { loadSiteModel } from "./site-model";
import type { SiteModel } from "./site-model";
import { gitSource } from "./source-git";
import { mddbSource } from "./source-mddb";

export type Origin = "git" | "mddb";

// ContentSource: one implementation per source. Each method validates its
// own paths and ids and answers with the operation's shape.
export interface ContentSource {
  origin: Origin;
  list(input: ListInput, budget?: number): Promise<ListPage>; // budget: requests left (a list of both sources)
  read(path: string): Promise<Article>;
  newPath(section: string, title: string | null | undefined): Promise<NewPath>;
  publish(body: unknown, req?: ChangeRequest): Promise<Published>; // req: the body already parsed
  history(path: string, cursor: string | null): Promise<RevisionPage>;
  readAt(path: string, id: string, cursor: string | null): Promise<RevisionText>;
  diff(input: DiffInput): Promise<RevisionDiff>;
}

// modelFor: the site model for MDDB's defaults when the repository is set
// up; null when it is not, or cannot be read (EDITOR_MDDB_COLLECTION then
// has to name the collection).
async function modelFor(ctx: OpContext): Promise<SiteModel | null> {
  const r = repositoryOf(ctx.env, ctx.deps);
  if (!r.ok) return null;
  try {
    const state = await loadSiteModel(r.repo.provider, r.repo.config, await r.repo.provider.branch(), ctx.env);
    return state.ok ? state.model : null;
  } catch (err) {
    if (err instanceof RepoError) return null;
    throw err;
  }
}

// openMddb: the MDDB source, or 503 mddb_not_configured naming what to set.
export async function openMddb(ctx: OpContext, model?: SiteModel | null): Promise<ContentSource> {
  const c = mddbConfigFrom(ctx.env, model === undefined ? await modelFor(ctx) : model);
  if (!c.ok) throw new OpError("mddb_not_configured", 503, c.problem);
  return mddbSource(ctx, c.config, repositoryOf(ctx.env, ctx.deps).ok);
}

// sourceFor: the source of one path (or new-path section).
export async function sourceFor(ctx: OpContext, path: unknown): Promise<ContentSource> {
  return isMddbPath(path) && mddbConfigured(ctx.env) ? openMddb(ctx) : gitSource(ctx);
}

const tagged = (p: ListPage): ListPage => ({ ...p, items: p.items.map((i) => ({ ...i, origin: "git" as const })) });

// listSources: the repository's files, then MDDB's articles, one cursor for
// both (the last path: an "mddb:" path means the repository part is done).
// Each part keeps its own order. MDDB is asked with the subrequests the
// repository part left; a page the files filled ends there, and the next one
// starts with MDDB.
export async function listSources(ctx: OpContext, input: ListInput): Promise<ListPage> {
  if (!mddbConfigured(ctx.env)) return gitSource(ctx).list(input);
  if (!repositoryOf(ctx.env, ctx.deps).ok && !(input.section && !isMddbPath(input.section))) return (await openMddb(ctx, null)).list(input);
  const s = await contentScope(ctx);
  const mddb = () => openMddb(ctx, s.model);
  const left = () => SUBREQUESTS - s.repo.provider.used();
  if (isMddbPath(input.section)) return (await mddb()).list(input, left());
  if (input.section) return tagged(await listContent(s, input));
  if (input.cursor && isMddbPath(decodeCursor(input.cursor))) return (await mddb()).list(input, left());
  const git = tagged(await listContent(s, input));
  if (git.nextCursor) return git;
  const room = limitOf(input.limit) - git.items.length;
  const source = await mddb();
  if (room < 1) return { ...git, nextCursor: encodeCursor(MDDB_PREFIX) };
  // At least one search: the file list keeps its title reads two short of
  // SUBREQUESTS (content-list.ts), so this stays within the plan's 50.
  const more = await source.list({ ...input, cursor: null, limit: String(room) }, Math.max(1, left()));
  return { items: [...git.items, ...more.items], nextCursor: more.nextCursor, pending: git.pending, truncated: git.truncated || more.truncated };
}

// publishThrough: one publication goes to one source. Images always go to
// the repository (mddb-publish.ts adds them in a commit of their own).
export async function publishThrough(ctx: OpContext, body: unknown): Promise<Published> {
  if (!mddbConfigured(ctx.env)) return gitSource(ctx).publish(body);
  const req = parseRequest(body, ctx.caller.perms);
  const articles = req.changes.filter((c) => !(c.op === "put" && "base64" in c.content));
  const inMddb = articles.filter((c) => (c.op === "move" ? isMddbPath(c.from) || isMddbPath(c.to) : isMddbPath(c.path)));
  if (!inMddb.length) return gitSource(ctx).publish(body, req);
  if (inMddb.length !== articles.length) throw new OpError("invalid_change", 400, "a publication goes to one source: the repository or MDDB");
  return (await openMddb(ctx)).publish(body, req);
}
