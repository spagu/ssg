// ssg editor-ui — portable repository layer: the provider interface, shared types, error codes and limits.
// SOURCE: workers/editor-ui/portable/; designer-ui carries byte-identical copies guarded by a test — edit the
// source, copy again. Files in portable/ import only each other (no npm, no node:), so they run unchanged in
// any Worker. One publication = one atomic commit (README decisions 2–4); a provider never forces a ref.

// RepoErrorCode: our own codes — a provider's status and body never reach the client (body text can echo
// request data). HTTP status a worker should answer with is in ERROR_STATUS.
export type RepoErrorCode =
  | "not_found" | "conflict" | "branch_protected" | "token_rejected" | "rate_limited" | "too_large"
  | "invalid_path" | "invalid_change" | "invalid_author" | "invalid_message" | "not_configured" | "provider_error";

export const ERROR_STATUS: Record<RepoErrorCode, number> = {
  not_found: 404, conflict: 409, branch_protected: 409, token_rejected: 502, rate_limited: 429, too_large: 413,
  invalid_path: 400, invalid_change: 400, invalid_author: 400, invalid_message: 400, not_configured: 503,
  provider_error: 502,
};

// Conflict: a file of the change differs on the branch from the version the caller worked on.
// currentBlobId null = the file does not exist on the branch (any more).
export interface Conflict { path: string; currentBlobId: string | null; headCommit: string }

// RepoError carries only safe data: a code, an optional short detail we wrote ourselves, the conflicting
// files, and for rate_limited the ISO time the provider says the limit resets.
export class RepoError extends Error {
  readonly code: RepoErrorCode;
  readonly detail?: string;
  readonly conflicts?: Conflict[];
  readonly retryAt?: string;
  constructor(code: RepoErrorCode, extra: { detail?: string; conflicts?: Conflict[]; retryAt?: string } = {}) {
    super(extra.detail ? `${code}: ${extra.detail}` : code);
    this.name = "RepoError";
    this.code = code;
    this.detail = extra.detail;
    this.conflicts = extra.conflicts;
    this.retryAt = extra.retryAt;
  }
  get status(): number { return ERROR_STATUS[this.code]; }
}

// FileContent: text is sent as UTF-8; base64 is sent to the provider as received (only checked by
// contentBytes, never re-encoded — the Free plan has 10 ms of CPU per request).
export type FileContent = { text: string } | { base64: string };

// Change: one file operation. baseBlobId is the version the caller worked on (from read()); null = the file
// must not exist yet. A move without content keeps the bytes (same blob, new path).
export type Change =
  | { op: "put"; path: string; content: FileContent; baseBlobId: string | null }
  | { op: "delete"; path: string; baseBlobId: string }
  | { op: "move"; from: string; to: string; baseBlobId: string; content?: FileContent };

export interface Author { name: string; email: string }
export interface CommitRequest { branch: string; message: string; author: Author; changes: Change[] }
// CommitResult: blobIds maps every path the change wrote to its new version id (null = removed).
export interface CommitResult { commitId: string; blobIds: Record<string, string | null>; attempts: number }

export type EntryType = "file" | "dir";
// TreeEntry: symlinks and submodules are never listed (they are not read nor written — GO-168).
export interface TreeEntry { path: string; type: EntryType; blobId: string; size?: number }
export interface TreeListing { entries: TreeEntry[]; truncated: boolean }
export interface RepoFile { path: string; bytes: Uint8Array; blobId: string }
export interface HistoryEntry { commitId: string; message: string; authorName: string; authorEmail: string; date: string }
export interface HistoryPage { items: HistoryEntry[]; nextCursor: string | null }

// RepoProvider: what a worker needs from a repository host. ref = a branch name or a commit id.
export interface RepoProvider {
  branch(): Promise<string>; // the configured branch, or the repository's default one
  head(branch: string): Promise<string>; // commit id at the tip of the branch
  tree(ref: string, prefix: string): Promise<TreeListing>;
  read(ref: string, path: string): Promise<RepoFile>;
  commit(req: CommitRequest): Promise<CommitResult>;
  history(path: string, cursor: string | null, ref?: string): Promise<HistoryPage>;
  readAt(commitId: string, path: string): Promise<RepoFile>;
}

// ChangeLimits: derived from the GO-168 measurement (below). A worker may lower or raise them
// (its plan), never above HARD_LIMITS.
export interface ChangeLimits { maxFilesPerChange: number; maxFileBytes: number; maxChangeBytes: number }
export const MB = 1024 * 1024;
// GO-168 measurement (2026-11-14, Node 25 on this code path, stubbed GitHub): ~2.5 ms CPU and ~2.4× the
// body in memory per MB of base64 payload; subrequests = files + 5 + directories + 6 per retry (≤ 3 tries).
// Free (10 ms CPU, 50 subrequests): 2 MB leaves room for auth and routing; 10 files in 10 directories with
// two lost races = 37 subrequests. Paid (30 s CPU, 128 MB): memory decides — 20 MB ≈ 50–60 MB peak.
export const PLAN_LIMITS: Record<"free" | "paid", ChangeLimits> = {
  free: { maxFilesPerChange: 10, maxFileBytes: 2 * MB, maxChangeBytes: 2 * MB },
  paid: { maxFilesPerChange: 50, maxFileBytes: 10 * MB, maxChangeBytes: 20 * MB },
};
export const HARD_LIMITS: ChangeLimits = { maxFilesPerChange: 200, maxFileBytes: 25 * MB, maxChangeBytes: 25 * MB };

// changeLimits: the plan's limits with a worker's overrides, each a positive integer clamped to HARD_LIMITS.
export function changeLimits(plan: "free" | "paid", overrides: Partial<Record<keyof ChangeLimits, unknown>> = {}): ChangeLimits {
  const out = { ...PLAN_LIMITS[plan] };
  for (const key of Object.keys(out) as (keyof ChangeLimits)[]) {
    const v = Number(overrides[key]);
    if (Number.isInteger(v) && v > 0) out[key] = Math.min(v, HARD_LIMITS[key]);
  }
  return out;
}

// contentBytes: the size of the bytes a FileContent stands for. Malformed base64 → invalid_change (the
// provider would reject it later, after a subrequest). Validation is the platform's atob — measured in GO-168
// at ~0.4 ms per MB, where an anchored regex over the same string cost ~4.8 ms (half the Free plan's 10 ms
// CPU). atob forgives whitespace and missing padding, so its length must also match the strict count.
export function contentBytes(c: FileContent): number {
  if ("text" in c) {
    if (typeof c.text !== "string") throw new RepoError("invalid_change", { detail: "text must be a string" });
    return new TextEncoder().encode(c.text).length;
  }
  const b = c.base64;
  const strict = typeof b === "string" && b.length % 4 === 0 ? (b.length / 4) * 3 - (b.endsWith("==") ? 2 : b.endsWith("=") ? 1 : 0) : -1;
  let decoded = -2;
  try { decoded = strict >= 0 ? atob(b).length : -2; } catch { /* not base64 */ }
  if (decoded !== strict) throw new RepoError("invalid_change", { detail: "content is not base64" });
  return strict;
}

// sanitizeAuthor: name and e-mail go into the commit's author line; a newline, "<", ">" or a control
// character there would forge another header or identity, so the name loses them and an e-mail with any
// of them is refused.
export function sanitizeAuthor(a: unknown): Author {
  const o = (a ?? {}) as Record<string, unknown>;
  const name = typeof o.name === "string" ? o.name.normalize("NFC").replace(/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}<>]/gu, " ").replace(/\s+/g, " ").trim().slice(0, 100) : "";
  const email = typeof o.email === "string" ? o.email.trim() : "";
  if (!name) throw new RepoError("invalid_author", { detail: "name is empty" });
  if (email.length > 254 || !/^[^\s<>@,;:"()\\\[\]\p{Cc}\p{Cf}]+@[^\s<>@,;:"()\\\[\]\p{Cc}\p{Cf}]+$/u.test(email)) {
    throw new RepoError("invalid_author", { detail: "e-mail is not valid" });
  }
  return { name, email };
}

// sanitizeMessage: CRLF → LF, control characters other than LF and tab dropped, at most 4000 characters.
export function sanitizeMessage(m: unknown): string {
  const s = typeof m === "string" ? m.replace(/\r\n?/g, "\n").replace(/[^\P{Cc}\n\t]/gu, "").trim().slice(0, 4000) : "";
  if (!s) throw new RepoError("invalid_message", { detail: "message is empty" });
  return s;
}

// validBranch: a conservative subset of git's ref-name rules (no "..", no "//", no leading "-" or "/",
// no ".lock" ending, printable ASCII without ~^:?*[\ and space).
export function validBranch(b: unknown): b is string {
  return typeof b === "string" && b.length > 0 && b.length <= 200 && /^[A-Za-z0-9._\/-]+$/.test(b)
    && !/^[-\/.]|\/$|\.\.|\/\/|\/\.|\.lock$|\.$/.test(b);
}

// validCommitId: a full SHA-1 or SHA-256 object id, lower-case hex.
export const validCommitId = (s: unknown): s is string => typeof s === "string" && /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(s);
