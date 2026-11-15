// GET {base}/api/revisions/{path}/diff?from=&to=&cursor= — what changed in
// one article between two revisions, or between a revision and the current
// version (`to` left out or `current`). Lines are compared in the Worker
// (diff.ts):
//   {path, from: {commitId, version, exists, lines, eol, finalNewline},
//    to: {commitId, current, version, exists, lines, eol, finalNewline},
//    identical, tooLarge, added, removed, hunks, download}
// A file absent on one side (before it was created, after it was removed)
// compares as empty with `exists: false`. Over diff.ts's limits, or over the
// provider's per-file limit, `tooLarge` says why and there are no hunks —
// `download` names the two routes that give each version whole.
// Both commit ids must be revisions of the file on the branch (revisions.ts),
// checked within one scan of the history.
//
// Requests to the repository host: the head, up to SCAN_PAGES history
// pages, then a folder listing and a blob per side.

import { contentPath } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { diffText } from "./diff";
import type { Hunk, SideInfo, TooLarge } from "./diff";
import type { Op } from "./ops";
import { PERM } from "./permissions";
import { readSide } from "./revision-read";
import type { Side } from "./revision-read";
import { commitParam, requireRevisions } from "./revisions";
import { sourceFor } from "./source";

export interface DiffSide extends SideInfo { commitId: string; version: string | null; exists: boolean }
export interface RevisionDiff {
  path: string;
  from: DiffSide;
  to: DiffSide & { current: boolean };
  identical: boolean;
  tooLarge: TooLarge | "file" | null; // "file": a side is over the provider's per-file limit
  added: number;
  removed: number;
  hunks: Hunk[];
  download: { from: string; to: string }; // routes relative to the base path
}

export type DiffInput = { path: string; from: string | null; to: string | null; cursor: string | null };

// diffRevisions: two versions of a repository file compared (source-git.ts).
export async function diffRevisions(s: ContentScope, input: DiffInput): Promise<RevisionDiff> {
  const path = contentPath(input.path, s, "read");
  const from = commitParam(input.from, "from");
  const current = input.to === null || input.to === "" || input.to === "current";
  const to = current ? s.model.head : commitParam(input.to, "to");
  await requireRevisions(s, path, current ? [from] : [from, to], input.cursor);
  const a = await readSide(s.repo.provider, from, path);
  const b = await readSide(s.repo.provider, to, path);
  const tooBig = a.tooLarge || b.tooLarge;
  const d = diffText(tooBig ? "" : a.text ?? "", tooBig ? "" : b.text ?? "");
  const side = (x: Side, commitId: string, info: SideInfo): DiffSide => ({ commitId, version: x.version, exists: x.text !== null, ...info });
  const segment = encodeURIComponent(path);
  const cursor = input.cursor ? `?cursor=${encodeURIComponent(input.cursor)}` : "";
  return {
    path,
    from: side(a, from, d.from),
    to: { ...side(b, to, d.to), current },
    // An empty file and no file are not the same; two unread sides are not known to be.
    identical: !tooBig && d.identical && (a.text === null) === (b.text === null),
    tooLarge: tooBig ? "file" : d.tooLarge,
    added: d.added,
    removed: d.removed,
    hunks: d.hunks,
    download: {
      from: `api/revisions/${segment}/${from}${cursor}`,
      to: current ? `api/content/${segment}` : `api/revisions/${segment}/${to}${cursor}`,
    },
  };
}

export const revisionDiffOp: Op<DiffInput, RevisionDiff> = {
  name: "revision_diff",
  permission: PERM.read,
  run: async (ctx, input) => (await sourceFor(ctx, input.path)).diff(input),
};
