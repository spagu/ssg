// ssg editor-ui — portable: one publication = one GitHub commit (README decisions 2–4), through the Git Data
// API: verify → blobs → tree on base_tree → commit → PATCH ref with force:false. Nothing is visible on the
// branch until the PATCH, so a failure at any earlier step leaves the branch untouched (at most unreachable
// blobs, which GitHub collects). A rejected PATCH means someone was faster: the change is retried on the new
// tip only if none of its files changed there; otherwise it is a conflict carrying the current version ids.
import {
  RepoError, contentBytes, sanitizeAuthor, sanitizeMessage, validBranch,
  type Change, type ChangeLimits, type CommitRequest, type CommitResult, type Conflict, type FileContent,
} from "./repo-provider";
import { normalizePath } from "./path-policy";
import { StaleRef, type githubHttp } from "./repo-github-http";
import { encodePath, headOf, lookup, rootTreeSha } from "./repo-github-tree";

type Http = ReturnType<typeof githubHttp>;
interface Check { path: string; base: string | null }
interface TreeItem { path: string; mode: "100644"; type: "blob"; sha: string | null }
const BLOB_ID = /^[0-9a-f]{40}$|^[0-9a-f]{64}$/;
const COMPARE_FILES_CAP = 300; // GitHub lists at most 300 files of a comparison

const bad = (detail: string) => new RepoError("invalid_change", { detail });
function checkedPath(p: unknown): string {
  const v = normalizePath(p);
  if (!v.ok || v.path !== p) throw new RepoError("invalid_path");
  return v.path;
}
const checkedBase = (b: unknown, nullable: boolean): string | null => {
  if ((b === null && nullable) || (typeof b === "string" && BLOB_ID.test(b))) return b as string | null;
  throw bad("bad baseBlobId");
};

// validate: shape, paths (already normalised by the caller's policy), one operation per path, sizes.
export function validate(changes: unknown, limits: ChangeLimits): { changes: Change[]; checks: Check[] } {
  if (!Array.isArray(changes) || changes.length === 0) throw bad("no changes");
  if (changes.length > limits.maxFilesPerChange) throw new RepoError("too_large", { detail: `at most ${limits.maxFilesPerChange} files per change` });
  const checks: Check[] = [];
  let total = 0;
  const size = (c: unknown) => {
    if (!c || typeof c !== "object" || (!("text" in c) && !("base64" in c))) throw bad("bad content");
    const n = contentBytes(c as FileContent);
    if (n > limits.maxFileBytes) throw new RepoError("too_large", { detail: `a file may have at most ${limits.maxFileBytes} bytes` });
    total += n;
  };
  for (const c of changes as Record<string, unknown>[]) {
    if (c?.op === "put") { size(c.content); checks.push({ path: checkedPath(c.path), base: checkedBase(c.baseBlobId, true) }); }
    else if (c?.op === "delete") checks.push({ path: checkedPath(c.path), base: checkedBase(c.baseBlobId, false) });
    else if (c?.op === "move") {
      if (c.content !== undefined) size(c.content);
      checks.push({ path: checkedPath(c.from), base: checkedBase(c.baseBlobId, false) }, { path: checkedPath(c.to), base: null });
    } else throw bad("unknown op");
  }
  if (total > limits.maxChangeBytes) throw new RepoError("too_large", { detail: `a change may have at most ${limits.maxChangeBytes} bytes` });
  if (new Set(checks.map((c) => c.path)).size !== checks.length) throw bad("a path appears twice");
  return { changes: changes as Change[], checks };
}

// conflictsAt compares every checked path at `head` with the version the caller worked on. A symlink or
// directory at a checked path is refused, never written through.
async function conflictsAt(http: Http, head: string, checks: Check[]): Promise<Conflict[]> {
  const found = await lookup(http, head, checks.map((c) => c.path));
  const out: Conflict[] = [];
  for (const c of checks) {
    const e = found.get(c.path) ?? null;
    if (e && e.type !== "file") throw new RepoError("invalid_path", { detail: "not a regular file" });
    if ((e?.sha ?? null) !== c.base) out.push({ path: c.path, currentBlobId: e?.sha ?? null, headCommit: head });
  }
  return out;
}

// conflictsSince: after a lost race, only files changed between the tip we verified and the new tip can
// conflict — one comparison instead of listing directories again. Not a plain fast-forward (history was
// rewritten) or a comparison GitHub may have cut → verify everything at the new tip.
async function conflictsSince(http: Http, verified: string, head: string, checks: Check[]): Promise<Conflict[]> {
  const cmp = await http.json({ path: `/compare/${verified}...${head}` });
  const files: unknown[] = Array.isArray(cmp?.files) ? cmp.files : [];
  if (cmp?.status !== "ahead" || files.length >= COMPARE_FILES_CAP) return conflictsAt(http, head, checks);
  const changed = new Map<string, string | null>();
  for (const f of files as { filename?: unknown; status?: unknown; sha?: unknown; previous_filename?: unknown }[]) {
    if (typeof f.previous_filename === "string") changed.set(f.previous_filename, null);
    if (typeof f.filename === "string") changed.set(f.filename, f.status === "removed" || typeof f.sha !== "string" ? null : f.sha);
  }
  return checks
    .filter((c) => changed.has(c.path) && changed.get(c.path) !== c.base)
    .map((c) => ({ path: c.path, currentBlobId: changed.get(c.path) as string | null, headCommit: head }));
}

async function createBlob(http: Http, content: FileContent): Promise<string> {
  const body = "text" in content ? { content: content.text, encoding: "utf-8" } : { content: content.base64, encoding: "base64" };
  const sha = (await http.json({ method: "POST", path: "/git/blobs", body }))?.sha;
  if (typeof sha !== "string" || !BLOB_ID.test(sha)) throw new RepoError("provider_error", { detail: "bad blob" });
  return sha;
}

// treeItems uploads the new contents (once, reused by every attempt) and lists the tree edits.
async function treeItems(http: Http, changes: Change[]): Promise<{ items: TreeItem[]; blobIds: Record<string, string | null> }> {
  const items: TreeItem[] = [];
  const blobIds: Record<string, string | null> = {};
  const put = (path: string, sha: string | null) => { items.push({ path, mode: "100644", type: "blob", sha }); blobIds[path] = sha; };
  for (const c of changes) {
    if (c.op === "put") put(c.path, await createBlob(http, c.content));
    else if (c.op === "delete") put(c.path, null);
    else { put(c.from, null); put(c.to, c.content ? await createBlob(http, c.content) : c.baseBlobId); }
  }
  return { items, blobIds };
}

async function created(http: Http, path: string, body: unknown): Promise<string> {
  const sha = (await http.json({ method: "POST", path, body }))?.sha;
  if (typeof sha !== "string" || !BLOB_ID.test(sha)) throw new RepoError("provider_error", { detail: "bad object" });
  return sha;
}

// commitChange publishes `req` as one commit on req.branch. maxAttempts bounds the retries after lost races.
export async function commitChange(http: Http, req: CommitRequest, limits: ChangeLimits, maxAttempts = 3): Promise<CommitResult> {
  const message = sanitizeMessage(req?.message);
  const author = sanitizeAuthor(req?.author);
  if (!validBranch(req.branch)) throw bad("bad branch");
  const { changes, checks } = validate(req.changes, limits);
  let verified: string | null = null;
  let upload: Awaited<ReturnType<typeof treeItems>> | null = null;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    const head = await headOf(http, req.branch);
    const conflicts = verified === null ? await conflictsAt(http, head, checks) : await conflictsSince(http, verified, head, checks);
    if (conflicts.length) throw new RepoError("conflict", { conflicts });
    verified = head;
    upload ??= await treeItems(http, changes);
    const tree = await created(http, "/git/trees", { base_tree: await rootTreeSha(http, head), tree: upload.items });
    const commitId = await created(http, "/git/commits", { message, tree, parents: [head], author });
    try {
      await http.json({ method: "PATCH", path: `/git/refs/heads/${encodePath(req.branch)}`, body: { sha: commitId, force: false } });
      return { commitId, blobIds: upload.blobIds, attempts: attempt };
    } catch (err) {
      if (!(err instanceof StaleRef)) throw err;
    }
  }
  // The branch kept moving without touching our files: nothing to choose between, the caller may retry.
  throw new RepoError("conflict", { detail: "branch is busy, try again", conflicts: [] });
}
