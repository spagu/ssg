// ssg editor-ui — portable: the GitHub RepoProvider (README decision 13: GitHub only in v1). Reads go
// through directory listings and blobs (repo-github-tree.ts), publications through the Git Data API
// (repo-github-commit.ts), every request through repo-github-http.ts (fixed API host, no redirects, our
// error codes). Construct one per request: it counts its own subrequests.
import {
  PLAN_LIMITS, RepoError, validBranch, validCommitId,
  type ChangeLimits, type HistoryEntry, type HistoryPage, type RepoFile, type RepoProvider,
} from "./repo-provider";
import type { RepoConfig } from "./repo-config";
import { githubHttp, RAW_ACCEPT, type HttpOptions } from "./repo-github-http";
import { checkRef, headOf, listTree, lookup } from "./repo-github-tree";
import { commitChange } from "./repo-github-commit";
import { normalizePath } from "./path-policy";

export interface GitHubOptions extends HttpOptions {
  limits?: ChangeLimits; // default PLAN_LIMITS.free; build with changeLimits(plan, overrides)
  maxAttempts?: number; // commit attempts after lost races (default 3)
  historyPageSize?: number; // default 30, at most 100 (GitHub's cap)
}

// githubProvider binds a RepoProvider to one repository. `used()` = subrequests sent so far.
export function githubProvider(config: RepoConfig, opts: GitHubOptions = {}): RepoProvider & { used(): number } {
  const http = githubHttp(config, opts);
  const limits = opts.limits ?? PLAN_LIMITS.free;
  const pageSize = Math.min(Math.max(Math.trunc(opts.historyPageSize ?? 30), 1), 100);
  let defaultBranch: string | null = config.branch;

  const filePath = (p: unknown) => {
    const v = normalizePath(p);
    if (!v.ok || v.path !== p) throw new RepoError("invalid_path");
    return v.path;
  };

  async function branch(): Promise<string> {
    if (defaultBranch) return defaultBranch;
    const b = (await http.json({ path: "" }))?.default_branch;
    if (!validBranch(b)) throw new RepoError("provider_error", { detail: "bad repository" });
    return (defaultBranch = b);
  }

  // read: the entry from its directory listing (symlink/submodule/dir → invalid_path), then its blob.
  async function read(ref: string, path: string): Promise<RepoFile> {
    const p = filePath(path);
    const e = (await lookup(http, checkRef(ref), [p])).get(p);
    if (!e) throw new RepoError("not_found");
    if (e.type !== "file") throw new RepoError("invalid_path", { detail: "not a regular file" });
    if ((e.size ?? 0) > limits.maxFileBytes) throw new RepoError("too_large");
    const bytes = await http.bytes({ path: `/git/blobs/${e.sha}`, accept: RAW_ACCEPT });
    return { path: p, bytes, blobId: e.sha };
  }

  // history: commits touching `path`, newest first; the cursor is GitHub's next page number.
  async function history(path: string, cursor: string | null, ref?: string): Promise<HistoryPage> {
    const p = filePath(path);
    if (cursor !== null && !/^[1-9][0-9]{0,5}$/.test(cursor)) throw new RepoError("invalid_change", { detail: "bad cursor" });
    const from = checkRef(ref ?? (await branch()));
    const q = `sha=${encodeURIComponent(from)}&path=${encodeURIComponent(p)}&per_page=${pageSize}&page=${cursor ?? "1"}`;
    const res = await http.send({ path: `/commits?${q}` });
    let list: unknown;
    try { list = await res.json(); } catch { throw new RepoError("provider_error", { detail: "bad response" }); }
    const items: HistoryEntry[] = (Array.isArray(list) ? list : []).flatMap((c: any) => {
      const a = c?.commit?.author;
      if (!validCommitId(c?.sha) || typeof c?.commit?.message !== "string") return [];
      return [{ commitId: c.sha, message: c.commit.message, authorName: String(a?.name ?? ""), authorEmail: String(a?.email ?? ""), date: String(a?.date ?? "") }];
    });
    return { items, nextCursor: nextPage(res.headers.get("link")) };
  }

  return {
    branch,
    head: async (b) => headOf(http, checkRef(b)),
    tree: async (ref, prefix) => listTree(http, checkRef(ref), prefix === "" ? "" : filePath(prefix)),
    read,
    readAt: async (commitId, path) => {
      if (!validCommitId(commitId)) throw new RepoError("invalid_change", { detail: "bad commit id" });
      return read(commitId, path);
    },
    commit: async (req) => commitChange(http, req, limits, opts.maxAttempts),
    history,
    used: () => http.state.used,
  };
}

// nextPage reads rel="next" from a Link header and keeps only its page number (never the URL itself).
export function nextPage(link: string | null): string | null {
  const next = (link ?? "").split(",").find((part) => /rel="next"/.test(part));
  const m = next && /[?&]page=([1-9][0-9]{0,5})(?:[&>])/.exec(next);
  return m ? m[1] : null;
}

