// GET {base}/api/revisions/{path}/{commit}?cursor= — the article as it was
// in one revision, and what the panel needs to bring it back:
//   {path, commitId, authorName, authorEmail, date, message, version, text,
//    current: {commit, version}, identical,
//    restore: {message, change: {op: "put", path, version, text}}}
// `commit` must be in the file's history on the branch (revisions.ts):
// anything else is 404 before a byte is read. `cursor` is the list page the
// revision was shown on.
//
// Restoring is no operation of its own (decision 6 — history is never
// rewritten): the panel loads `text` as a draft of the current file and
// publishes it with POST /api/changes — `restore.change` is that change,
// based on the current version, so a file that moved on since is a normal
// conflict; `restore.message` is the suggested commit message. The result
// is a new commit on top of the branch.
//
// Requests to the repository host: the head, up to SCAN_PAGES history pages,
// the folder listing and blob at the revision, the same at the head.

import { RepoError } from "../portable/repo-provider";
import type { RepoProvider } from "../portable/repo-provider";
import { contentPath } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { decodeText } from "./content-meta";
import { OpError } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { commitParam, requireRevisions } from "./revisions";
import { sourceFor } from "./source";

// Side: a file at one commit. text null = not there; tooLarge = over the
// provider's per-file limit (not read).
export interface Side { text: string | null; version: string | null; tooLarge: boolean }

// readSide reads `path` at `ref` (already checked to be the head or a
// revision of the file); 422 not_text for bytes that are not UTF-8.
export async function readSide(provider: RepoProvider, ref: string, path: string): Promise<Side> {
  try {
    const f = await provider.read(ref, path);
    const text = decodeText(f.bytes);
    if (text === null) throw new OpError("not_text", 422, "the file is not UTF-8 text", { commit: ref });
    return { text, version: f.blobId, tooLarge: false };
  } catch (err) {
    if (err instanceof RepoError && err.code === "not_found") return { text: null, version: null, tooLarge: false };
    if (err instanceof RepoError && err.code === "too_large") return { text: null, version: null, tooLarge: true };
    throw err;
  }
}

export interface RevisionText {
  path: string;
  commitId: string;
  authorName: string;
  authorEmail: string;
  date: string;
  message: string; // the whole commit message
  version: string; // the file's blob id in this revision
  text: string;
  current: { commit: string; version: string | null }; // version null: the file is not on the branch now
  identical: boolean; // this revision is the current version
  restore: { message: string; change: { op: "put"; path: string; version: string | null; text: string } };
}

// restoreMessage: "Restore hello.md to 1a2b3c4 of 2026-01-01" (the first line
// of a publication may be 200 characters).
export const restoreMessage = (path: string, commitId: string, date: string): string =>
  ["Restore", path.slice(path.lastIndexOf("/") + 1), "to", commitId.slice(0, 7), date ? "of " + date.slice(0, 10) : ""].filter(Boolean).join(" ").slice(0, 200);

// readRevision: a repository file at one commit of its history (source-git.ts).
export async function readRevision(s: ContentScope, raw: string, rawCommit: string, cursor: string | null): Promise<RevisionText> {
  const path = contentPath(raw, s, "read");
  const commit = commitParam(rawCommit, "commit");
  const entry = (await requireRevisions(s, path, [commit], cursor)).get(commit)!;
  const old = await readSide(s.repo.provider, commit, path);
  if (old.tooLarge) throw new OpError("too_large", 413, "this revision is larger than the editor reads");
  if (old.text === null) throw new OpError("not_in_revision", 404, "the file was removed in this revision", { commit });
  const now = await readSide(s.repo.provider, s.model.head, path);
  if (now.tooLarge) throw new OpError("too_large", 413, "the current version is larger than the editor reads");
  return {
    path, commitId: commit, authorName: entry.authorName, authorEmail: entry.authorEmail, date: entry.date, message: entry.message,
    version: old.version!, text: old.text,
    current: { commit: s.model.head, version: now.version },
    identical: now.version === old.version,
    restore: { message: restoreMessage(path, commit, entry.date), change: { op: "put", path, version: now.version, text: old.text } },
  };
}

export const revisionReadOp: Op<{ path: string; commit: string; cursor: string | null }, RevisionText> = {
  name: "revision_read",
  permission: PERM.read,
  run: async (ctx, input) => (await sourceFor(ctx, input.path)).readAt(input.path, input.commit, input.cursor),
};
