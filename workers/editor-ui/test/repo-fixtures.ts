// Shared setup for the repository-layer tests: a fake GitHub seeded with a small site, a config pointing
// at it and a provider bound to the fake's fetch.
import { FakeGitHub, TOKEN } from "./fake-github";
import { repoConfigFrom, type RepoConfig } from "../portable/repo-config";
import { githubProvider, type GitHubOptions } from "../portable/repo-github";
import { RepoError, type Author } from "../portable/repo-provider";

export const AUTHOR: Author = { name: "Ada Writer", email: "ada@example.com" };
export const ENV = { EDITOR_GIT_REPO: "github.com/acme/site", EDITOR_GIT_BRANCH: "main", EDITOR_GIT_TOKEN: TOKEN };

export function config(env: Record<string, unknown> = ENV): RepoConfig {
  const r = repoConfigFrom(env, "EDITOR");
  if (!r.ok) throw new Error(r.problem);
  return r.config;
}

export async function setup(opts: GitHubOptions = {}, env: Record<string, unknown> = ENV) {
  const gh = new FakeGitHub();
  await gh.store.commitFiles("main", {
    "content/posts/hello.md": "# Hello\n",
    "content/posts/second.md": "# Second\n",
    "content/pages/about.md": "# About\n",
    "static/images/logo.png": "PNG",
    ".ssg.yaml": "title: Site\n",
  });
  const provider = githubProvider(config(env), { fetch: gh.fetch, ...opts });
  return { gh, provider, blob: (path: string) => gh.store.blobIdAt("main", path) };
}

// rejects: awaits a promise that must fail with a RepoError and returns it.
export async function rejects(p: Promise<unknown>): Promise<RepoError> {
  try {
    await p;
  } catch (err) {
    if (err instanceof RepoError) return err;
    throw err;
  }
  throw new Error("expected a RepoError");
}
