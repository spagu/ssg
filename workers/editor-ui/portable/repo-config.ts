// ssg editor-ui — portable: repository configuration from a Worker's env. The prefix is the caller's
// (EDITOR, DESIGNER): <PREFIX>_GIT_REPO, <PREFIX>_GIT_BRANCH, <PREFIX>_GIT_TOKEN (a secret). Anything missing
// or malformed → a problem the worker answers with 503; never half a configuration. The API host comes from
// the provider table below, never from the user's configuration, so a configured value cannot send the
// token anywhere else.
import { validBranch } from "./repo-provider";

export type ProviderName = "github";
// PROVIDERS: repository host → provider and its API base. v1: GitHub only (README decision 13).
export const PROVIDERS: Record<string, { provider: ProviderName; apiBase: string }> = {
  "github.com": { provider: "github", apiBase: "https://api.github.com" },
};

// RepoConfig: branch null = the repository's default branch (the provider looks it up).
export interface RepoConfig { provider: ProviderName; apiBase: string; owner: string; repo: string; branch: string | null; token: string }
// ConfigProblem names the variable and what is wrong with it — never its value.
export type RepoConfigResult = { ok: true; config: RepoConfig } | { ok: false; problem: string };

const OWNER = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$/;
const REPO = /^[A-Za-z0-9._-]{1,100}$/;
// A token is one header value: printable ASCII without spaces (a newline would add a header).
const TOKEN = /^[\x21-\x7e]{8,512}$/;

// parseRepo reads "github.com/<owner>/<repo>", also with "https://" in front and/or ".git" after.
export function parseRepo(raw: string): { host: string; owner: string; repo: string } | null {
  const m = /^(?:https:\/\/)?([A-Za-z0-9.-]+)\/([^/]+)\/([^/]+?)(?:\.git)?\/?$/.exec(raw.trim());
  if (!m || !OWNER.test(m[2]) || !REPO.test(m[3]) || m[3] === "." || m[3] === "..") return null;
  return { host: m[1].toLowerCase(), owner: m[2], repo: m[3] };
}

// repoConfigFrom reads the three variables for `prefix` (upper-case letters, digits, underscore).
export function repoConfigFrom(env: Record<string, unknown>, prefix: string): RepoConfigResult {
  if (!/^[A-Z][A-Z0-9_]*$/.test(prefix)) return { ok: false, problem: "invalid configuration prefix" };
  const get = (name: string) => {
    const v = env[`${prefix}_${name}`];
    return typeof v === "string" ? v.trim() : "";
  };
  const repoRaw = get("GIT_REPO"), branch = get("GIT_BRANCH"), token = get("GIT_TOKEN");
  if (!repoRaw) return { ok: false, problem: `${prefix}_GIT_REPO is not set` };
  const parsed = parseRepo(repoRaw);
  if (!parsed) return { ok: false, problem: `${prefix}_GIT_REPO must look like github.com/<owner>/<repo>` };
  const host = PROVIDERS[parsed.host];
  if (!host) return { ok: false, problem: `${prefix}_GIT_REPO: only github.com is supported` };
  if (branch && !validBranch(branch)) return { ok: false, problem: `${prefix}_GIT_BRANCH is not a valid branch name` };
  if (!token) return { ok: false, problem: `${prefix}_GIT_TOKEN is not set (npx wrangler secret put ${prefix}_GIT_TOKEN)` };
  if (!TOKEN.test(token)) return { ok: false, problem: `${prefix}_GIT_TOKEN is malformed` };
  return { ok: true, config: { ...host, owner: parsed.owner, repo: parsed.repo, branch: branch || null, token } };
}
