// POST {base}/api/changes — one publication = one commit on the configured
// branch with every file of the change (README of the round, decisions 2–5).
// The body is change-input.ts's; the answer:
//   201 {commitId, url, branch, files: [{path, version}]}   (version null = removed)
//   409 {error: "conflict", files: [{path, yours, theirs, theirVersion}]}
//   409 {error: "branch_busy"} — the branch kept moving without touching
//       these files; nothing to choose between, publish again
//   400 invalid_path | invalid_change | invalid_message | fields_refused | invalid_image, 403, 413
// Images (media-change.ts) already on the branch with the same bytes are
// listed in `files` with their version and not written again; when nothing
// else is left, commitId is the branch head and no commit is made.
//
// A conflict loses nothing: `yours` is the text this change would have
// written (for a form edit: your fields and body applied to their version),
// `theirs` the text on the branch now (null: the file is gone, or exists
// already for a new file the other way round), `theirVersion` its version to
// publish over once the person has chosen. When the subrequest budget cannot
// fit reading `theirs`, `theirsUnread` says why and contentRead fetches it.

import { RepoError } from "../portable/repo-provider";
import type { Change, Conflict } from "../portable/repo-provider";
import { PROVIDERS } from "../portable/repo-config";
import type { RepoConfig } from "../portable/repo-config";
import { authorOf } from "./author";
import { parseRequest } from "./change-input";
import type { Content, Planned } from "./change-input";
import { READ_COST } from "./content-list";
import { decodeText } from "./content-meta";
import { contentPath, contentScope } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { NEW_FILE, applyEdit } from "./frontmatter";
import { checkImageLimits, deletablePath, imageChange } from "./media-change";
import { OpError } from "./ops";
import type { Op } from "./ops";
import type { Rebuild } from "./rebuild";
import { PERM } from "./permissions";
import { SUBREQUESTS } from "./repo";
import { publishThrough } from "./source";

// origin, rebuild, notice: only on an MDDB publication (mddb-publish.ts).
export interface Published {
  commitId: string; url: string; branch: string; files: { path: string; version: string | null }[];
  origin?: "mddb"; rebuild?: Rebuild; notice?: string;
}
export interface ConflictFile { path: string; yours: string | null; theirs: string | null; theirVersion: string | null; theirsUnread?: "budget" | "not_text" | "unavailable" }

// MAX_FORM_EDITS: a form edit reads the file first (two subrequests), so a
// publication carries a few of them at most; whole texts have no such cost.
export const MAX_FORM_EDITS = 5;

// commitUrl: the commit's page at the repository host.
export function commitUrl(config: RepoConfig, commitId: string): string {
  const host = Object.keys(PROVIDERS).find((h) => PROVIDERS[h].provider === config.provider);
  return `https://${host}/${config.owner}/${config.repo}/commit/${commitId}`;
}

// current: the file on the branch now, or null when it is not there.
async function current(s: ContentScope, path: string, ref: string): Promise<{ blobId: string; text: string | null } | null> {
  try {
    const f = await s.repo.provider.read(ref, path);
    return { blobId: f.blobId, text: decodeText(f.bytes) };
  } catch (err) {
    if (err instanceof RepoError && err.code === "not_found") return null;
    throw err;
  }
}

// kept: images of the change already on the branch with the same bytes (media-change.ts).
interface Resolved { changes: Change[]; yours: Map<string, string | null>; early: ConflictFile[]; kept: { path: string; version: string }[] }

// textOf: the new text of a change. A form edit is applied to the file it
// was made on — read now; a different version is a conflict found early.
async function textOf(s: ContentScope, path: string, version: string | null, content: Content, early: ConflictFile[]): Promise<string | null> {
  if ("text" in content) return content.text;
  const cur = version === null ? null : await current(s, path, s.model.head);
  if (version !== null && cur?.blobId !== version) {
    const theirs = cur?.text ?? null;
    // Your edit on their version; when it does not apply there (their file
    // lost its frontmatter), on a fresh block — so nothing typed is lost.
    let mine = applyEdit(theirs ?? NEW_FILE, content.edit);
    if (mine.refused.length) mine = applyEdit(NEW_FILE, content.edit);
    early.push({ path, yours: mine.refused.length ? null : mine.text, theirs, theirVersion: cur?.blobId ?? null });
    return null;
  }
  if (cur && cur.text === null) throw new OpError("not_text", 422, "the file is not UTF-8 text; send its whole text instead", { path });
  const r = applyEdit(cur?.text ?? NEW_FILE, content.edit);
  if (r.refused.length) throw new OpError("fields_refused", 400, "these frontmatter keys cannot be changed from the form", { path, refused: r.refused });
  return r.text;
}

async function resolve(s: ContentScope, planned: Planned[]): Promise<Resolved> {
  const out: Resolved = { changes: [], yours: new Map(), early: [], kept: [] };
  checkImageLimits(planned);
  const edits = planned.filter((p) => p.op !== "delete" && p.content !== null && "edit" in p.content).length;
  if (edits > MAX_FORM_EDITS) throw new OpError("too_large", 413, `at most ${MAX_FORM_EDITS} form edits per publication; send whole texts instead`);
  for (const p of planned) {
    if (p.op === "delete") {
      const path = deletablePath(p.path, s);
      out.changes.push({ op: "delete", path, baseBlobId: p.version });
      out.yours.set(path, null);
    } else if (p.op === "put") {
      const content = p.content;
      if ("base64" in content) {
        // The same image pasted twice into one publication is one file.
        const r = await imageChange(s, p.path, content.base64);
        const path = "kept" in r ? r.kept.path : r.change.path;
        if (out.yours.has(path)) continue;
        if ("kept" in r) out.kept.push(r.kept);
        else out.changes.push(r.change);
        out.yours.set(path, null);
        continue;
      }
      const path = contentPath(p.path, s, "write");
      const text = await textOf(s, path, p.version, content, out.early);
      if (text !== null) out.changes.push({ op: "put", path, content: { text }, baseBlobId: p.version });
      out.yours.set(path, text);
    } else {
      const from = contentPath(p.from, s, "write");
      const to = contentPath(p.to, s, "write");
      const text = p.content ? await textOf(s, from, p.version, p.content, out.early) : null;
      out.changes.push({ op: "move", from, to, baseBlobId: p.version, ...(text === null ? {} : { content: { text } }) });
      out.yours.set(from, null).set(to, text);
    }
  }
  return out;
}

// theirsOf: the text on the branch for one conflicting file, within the budget.
async function theirsOf(s: ContentScope, c: Conflict): Promise<Pick<ConflictFile, "theirs" | "theirsUnread">> {
  if (c.currentBlobId === null) return { theirs: null };
  if (s.repo.provider.used() + READ_COST > SUBREQUESTS) return { theirs: null, theirsUnread: "budget" };
  try {
    const theirs = decodeText((await s.repo.provider.read(c.headCommit, c.path)).bytes);
    return theirs === null ? { theirs, theirsUnread: "not_text" } : { theirs };
  } catch (err) {
    if (!(err instanceof RepoError)) throw err;
    return { theirs: null, theirsUnread: "unavailable" };
  }
}

const conflict = (files: ConflictFile[]) =>
  new OpError("conflict", 409, "these files changed on the branch since they were opened", { files });

export async function conflictOf(s: ContentScope, err: RepoError, yours: Map<string, string | null>): Promise<OpError> {
  if (!err.conflicts?.length) return new OpError("branch_busy", 409, "the branch kept moving; publish again");
  const files: ConflictFile[] = [];
  for (const c of err.conflicts) files.push({ path: c.path, yours: yours.get(c.path) ?? null, theirVersion: c.currentBlobId, ...(await theirsOf(s, c)) });
  return conflict(files);
}

export async function publish(ctx: Parameters<Op["run"]>[0], body: unknown): Promise<Published> {
  const req = parseRequest(body, ctx.caller.perms);
  const author = authorOf(ctx.caller);
  const s = await contentScope(ctx);
  const { changes, yours, early, kept } = await resolve(s, req.changes);
  if (early.length) throw conflict(early);
  // Only images already on the branch: nothing to commit, the head stands.
  if (!changes.length) return { commitId: s.model.head, url: commitUrl(s.repo.config, s.model.head), branch: s.branch, files: kept };
  try {
    const r = await s.repo.provider.commit({ branch: s.branch, message: req.message, author, changes });
    const files = [...Object.entries(r.blobIds).map(([path, version]) => ({ path, version })), ...kept];
    return { commitId: r.commitId, url: commitUrl(s.repo.config, r.commitId), branch: s.branch, files };
  } catch (err) {
    if (err instanceof RepoError && err.code === "conflict") throw await conflictOf(s, err, yours);
    throw err;
  }
}

export const changePublishOp: Op<unknown, Published> = {
  name: "change_publish",
  permission: PERM.publish,
  run: (ctx, body) => publishThrough(ctx, body),
};
