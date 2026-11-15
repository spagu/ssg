// GET {base}/api/revisions/{path}?cursor= — the commits that changed one
// article on the branch, newest first (README of the round, decision 6:
// revisions are the repository's history):
//   {path, items: [{commitId, authorName, authorEmail, date, message}], nextCursor}
// `message` is the commit message's first line. A rename ends the list at
// the commit that created the file under this name. A "previously as …"
// hint needs the repository host to report renames; GitHub's commit list
// filtered by path does not, so v1 has none — the editor keeps no history
// of its own (a provider that reports renames would add it to HistoryPage).
//
// findRevisions is the membership check of the other revision routes: a
// commit id is read only after it was found in this file's history on the
// branch, so an id from another branch, a fork or another file is a 404 and
// no object is ever read by an id a caller made up.
//
// Requests to the repository host: the head (site model), one per history
// page.

import { RepoError, validCommitId } from "../portable/repo-provider";
import type { HistoryEntry, HistoryPage, RepoProvider } from "../portable/repo-provider";
import { contentPath } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { OpError } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { sourceFor } from "./source";

export interface Revision { commitId: string; authorName: string; authorEmail: string; date: string; message: string }
export interface RevisionPage { path: string; items: Revision[]; nextCursor: string | null }

// SCAN_PAGES: history pages one check may read (30 commits each on GitHub).
// The panel passes the cursor of the page it listed the commit on, so the
// commit is on the first page read — or on the next ones when commits to the
// file landed since. Older without a cursor → 404, never a longer walk.
export const SCAN_PAGES = 3;

const firstLine = (m: string) => m.split("\n", 1)[0].trim();
const revisionOf = (e: HistoryEntry): Revision => ({ commitId: e.commitId, authorName: e.authorName, authorEmail: e.authorEmail, date: e.date, message: firstLine(e.message) });

// historyPage: one page of the file's history at `head`; a cursor the host
// refuses is the caller's mistake (400 bad_cursor).
async function historyPage(provider: RepoProvider, path: string, cursor: string | null, head: string): Promise<HistoryPage> {
  try {
    return await provider.history(path, cursor, head);
  } catch (err) {
    if (err instanceof RepoError && err.code === "invalid_change") throw new OpError("bad_cursor", 400, "not a cursor of this file's history");
    throw err;
  }
}

// findRevisions: the history entries of `ids` found within SCAN_PAGES pages
// from `cursor`. Missing ids are absent from the map.
export async function findRevisions(s: ContentScope, path: string, ids: string[], cursor: string | null): Promise<Map<string, HistoryEntry>> {
  const found = new Map<string, HistoryEntry>();
  let at = cursor;
  for (let i = 0; i < SCAN_PAGES; i++) {
    const page = await historyPage(s.repo.provider, path, at, s.model.head);
    for (const e of page.items) if (ids.includes(e.commitId)) found.set(e.commitId, e);
    if (page.nextCursor === null || ids.every((id) => found.has(id))) break;
    at = page.nextCursor;
  }
  return found;
}

// commitParam: a full commit id, else 400 invalid_commit.
export function commitParam(raw: unknown, name: string): string {
  if (!validCommitId(raw)) throw new OpError("invalid_commit", 400, `${name} must be a full commit id`, { param: name });
  return raw;
}

// requireRevisions: findRevisions, or 404 revision_not_found naming the first missing id.
export async function requireRevisions(s: ContentScope, path: string, ids: string[], cursor: string | null): Promise<Map<string, HistoryEntry>> {
  const found = await findRevisions(s, path, ids, cursor);
  const missing = ids.find((id) => !found.has(id));
  if (missing) {
    throw new OpError("revision_not_found", 404, "not a revision of this file on the branch (or older than the pages searched: pass the list's cursor)", { commit: missing });
  }
  return found;
}

// listRevisions: one page of a repository file's history (source-git.ts).
export async function listRevisions(s: ContentScope, raw: string, cursor: string | null): Promise<RevisionPage> {
  const path = contentPath(raw, s, "read");
  const page = await historyPage(s.repo.provider, path, cursor, s.model.head);
  return { path, items: page.items.map(revisionOf), nextCursor: page.nextCursor };
}

export const revisionsListOp: Op<{ path: string; cursor: string | null }, RevisionPage> = {
  name: "revisions_list",
  permission: PERM.read,
  run: async (ctx, input) => (await sourceFor(ctx, input.path)).history(input.path, input.cursor),
};
