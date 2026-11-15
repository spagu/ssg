// ssg editor-ui — portable: reading GitHub trees. A path is resolved through its directory's listing,
// which carries each entry's type: symlinks and submodules are seen as such and never followed (the
// contents endpoint for a single path would follow a symlink to its target — so it is not used for files;
// bytes come from the blob, which is content-addressed). A listing is capped at 1000 entries by GitHub,
// so a full one is re-read through the Git trees endpoint, which has no such cap.
import { RepoError, validBranch, validCommitId, type TreeListing } from "./repo-provider";
import type { githubHttp } from "./repo-github-http";

type Http = ReturnType<typeof githubHttp>;
export type Kind = "file" | "dir" | "symlink" | "submodule";
export interface DirEntry { type: Kind; sha: string; size?: number }
export const CONTENTS_CAP = 1000; // GitHub's documented directory-listing cap

export const encodePath = (p: string) => p.split("/").map(encodeURIComponent).join("/");
const parentOf = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "");
const nameOf = (p: string) => p.slice(p.lastIndexOf("/") + 1);
// modeKind maps a git tree mode (Git trees endpoint) to our kinds.
const modeKind = (mode: string): Kind => (mode === "040000" ? "dir" : mode === "120000" ? "symlink" : mode === "160000" ? "submodule" : "file");

// checkRef: a branch name or a commit id, nothing that could change the URL's meaning.
export function checkRef(ref: unknown): string {
  if (!validBranch(ref)) throw new RepoError("invalid_change", { detail: "bad ref" });
  return ref;
}

// rootTreeSha: the tree of a commit id, or of a branch's tip.
export async function rootTreeSha(http: Http, ref: string): Promise<string> {
  const commit = validCommitId(ref) ? ref : await headOf(http, ref);
  const sha = (await http.json({ path: `/git/commits/${commit}` }))?.tree?.sha;
  if (!validCommitId(sha)) throw new RepoError("provider_error", { detail: "bad commit" });
  return sha;
}

// headOf: the commit id at the tip of `branch`.
export async function headOf(http: Http, branch: string): Promise<string> {
  const sha = (await http.json({ path: `/git/ref/heads/${encodePath(branch)}` }))?.object?.sha;
  if (!validCommitId(sha)) throw new RepoError("provider_error", { detail: "bad ref" });
  return sha;
}

// treeEntries reads one tree object (optionally recursive) as [relative path, entry] pairs.
async function treeEntries(http: Http, sha: string, recursive: boolean) {
  const res = await http.json({ path: `/git/trees/${sha}${recursive ? "?recursive=1" : ""}` });
  const list: unknown[] = Array.isArray(res?.tree) ? res.tree : [];
  const out: [string, DirEntry][] = [];
  for (const e of list as { path?: unknown; mode?: unknown; sha?: unknown; size?: unknown }[]) {
    if (typeof e.path !== "string" || typeof e.mode !== "string" || typeof e.sha !== "string") continue;
    out.push([e.path, { type: modeKind(e.mode), sha: e.sha, size: typeof e.size === "number" ? e.size : undefined }]);
  }
  return { out, truncated: res?.truncated === true };
}

// dirEntries lists one directory at `ref` (name → entry), or null when it does not exist.
export async function dirEntries(http: Http, ref: string, dir: string): Promise<Map<string, DirEntry> | null> {
  let res: unknown;
  try {
    res = await http.json({ path: `/contents${dir ? "/" + encodePath(dir) : ""}?ref=${encodeURIComponent(ref)}` });
  } catch (err) {
    if (err instanceof RepoError && err.code === "not_found") return null;
    throw err;
  }
  if (!Array.isArray(res)) return null; // the path is a file, not a directory
  if (res.length < CONTENTS_CAP) {
    const map = new Map<string, DirEntry>();
    for (const e of res as { name?: unknown; type?: unknown; sha?: unknown; size?: unknown }[]) {
      if (typeof e.name !== "string" || typeof e.sha !== "string") continue;
      const type: Kind = e.type === "dir" || e.type === "symlink" || e.type === "submodule" ? e.type : "file";
      map.set(e.name, { type, sha: e.sha, size: typeof e.size === "number" ? e.size : undefined });
    }
    return map;
  }
  const sha = await dirTreeSha(http, ref, dir);
  return sha === null ? null : new Map((await treeEntries(http, sha, false)).out);
}

// dirTreeSha: the tree object of a directory (root: the commit's tree), or null when absent.
async function dirTreeSha(http: Http, ref: string, dir: string): Promise<string | null> {
  if (!dir) return rootTreeSha(http, ref);
  const entry = (await dirEntries(http, ref, parentOf(dir)))?.get(nameOf(dir));
  return entry?.type === "dir" ? entry.sha : null;
}

// lookup resolves several paths at `ref`, one listing per distinct directory; absent → null.
export async function lookup(http: Http, ref: string, paths: string[]): Promise<Map<string, DirEntry | null>> {
  const dirs = new Map<string, Map<string, DirEntry> | null>();
  const out = new Map<string, DirEntry | null>();
  for (const p of paths) {
    const dir = parentOf(p);
    if (!dirs.has(dir)) dirs.set(dir, await dirEntries(http, ref, dir));
    out.set(p, dirs.get(dir)?.get(nameOf(p)) ?? null);
  }
  return out;
}

// listTree: every file and directory under `prefix` ("" = the whole repository), without symlinks and
// submodules. truncated = GitHub cut the recursive listing (100 000 entries or 7 MB).
export async function listTree(http: Http, ref: string, prefix: string): Promise<TreeListing> {
  const sha = await dirTreeSha(http, ref, prefix);
  if (sha === null) throw new RepoError("not_found");
  const { out, truncated } = await treeEntries(http, sha, true);
  const entries = out
    .filter(([, e]) => e.type === "file" || e.type === "dir")
    .map(([path, e]) => ({ path: prefix ? `${prefix}/${path}` : path, type: e.type as "file" | "dir", blobId: e.sha, ...(e.size === undefined ? {} : { size: e.size }) }));
  return { entries, truncated };
}
