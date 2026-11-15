// In-process fake of the GitHub REST endpoints the portable provider uses — no network, no real token.
// Behaviour follows GitHub's REST documentation as verified for GO-168 (2026-11-14): contents listings
// capped at 1000, compare files capped at 300, PATCH ref refusing a non-fast-forward with 422, protected
// branch refusing with 422, rate limits with x-ratelimit-* headers. Every request is recorded; hooks let a
// test run "another publication" exactly between our commit and our PATCH (the race window).
import { GitStore, type Flat } from "./fake-git-store";

export const API = "https://api.github.com";
export const TOKEN = "ghp_FAKEtoken0123456789abcdefSECRET";
export interface Recorded { method: string; url: string; headers: Record<string, string>; body: string | undefined; redirect: string | undefined }

const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });
const b64decode = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

export class FakeGitHub {
  store = new GitStore();
  requests: Recorded[] = [];
  patches: { sha: string; force: unknown }[] = [];
  protectedBranches = new Set<string>();
  contentsCap = 1000;
  compareCap = 300;
  defaultBranch = "main";
  // override: answer a request before the fake does (inject errors, redirects, rate limits).
  override: ((method: string, path: string) => Response | undefined) | null = null;
  // beforePatch: runs once per PATCH, before the ref check — the race window.
  beforePatch: (() => Promise<void>) | null = null;

  owner: string;
  repo: string;
  constructor(owner = "acme", repo = "site") { this.owner = owner; this.repo = repo; }
  get base() { return `${API}/repos/${this.owner}/${this.repo}`; }

  fetch = async (url: string, init: RequestInit = {}): Promise<Response> => {
    const headers = Object.fromEntries(new Headers(init.headers).entries());
    const body = typeof init.body === "string" ? init.body : undefined;
    this.requests.push({ method: init.method ?? "GET", url, headers, body, redirect: init.redirect });
    if (!url.startsWith(this.base)) return json(404, { message: "Not Found" });
    if (headers.authorization !== `Bearer ${TOKEN}`) return json(401, { message: "Bad credentials" });
    const u = new URL(url);
    const path = decodeURIComponent(u.pathname.slice(new URL(this.base).pathname.length));
    const method = init.method ?? "GET";
    const forced = this.override?.(method, path);
    if (forced) return forced;
    return this.route(method, path, u.searchParams, body ? JSON.parse(body) : undefined, headers.accept);
  };

  private async route(method: string, path: string, q: URLSearchParams, body: any, accept: string | undefined): Promise<Response> {
    const s = this.store;
    let m: RegExpExecArray | null;
    if (method === "GET" && path === "") return json(200, { default_branch: this.defaultBranch });
    if (method === "GET" && (m = /^\/git\/ref\/heads\/(.+)$/.exec(path))) {
      const sha = s.refs.get(m[1]);
      return sha ? json(200, { ref: `refs/heads/${m[1]}`, object: { sha, type: "commit" } }) : json(404, { message: "Not Found" });
    }
    if (method === "GET" && (m = /^\/git\/commits\/([0-9a-f]+)$/.exec(path))) {
      const c = s.commits.get(m[1]);
      return c ? json(200, { sha: m[1], tree: { sha: c.tree }, parents: c.parents.map((sha) => ({ sha })) }) : json(404, { message: "Not Found" });
    }
    if (method === "GET" && (m = /^\/contents(?:\/(.*))?$/.exec(path))) return this.contents(m[1] ?? "", q.get("ref") ?? this.defaultBranch);
    if (method === "GET" && (m = /^\/git\/trees\/([0-9a-f]+)$/.exec(path))) return this.getTree(m[1], q.has("recursive"));
    if (method === "GET" && (m = /^\/git\/blobs\/([0-9a-f]+)$/.exec(path))) {
      const b = s.blobs.get(m[1]);
      if (!b) return json(404, { message: "Not Found" });
      return accept?.includes("raw") ? new Response(b, { status: 200 }) : json(200, { sha: m[1], size: b.length });
    }
    if (method === "POST" && path === "/git/blobs") {
      const bytes = body.encoding === "base64" ? b64decode(body.content) : new TextEncoder().encode(body.content);
      return json(201, { sha: await s.putBlob(bytes) });
    }
    if (method === "POST" && path === "/git/trees") return this.createTree(body);
    if (method === "POST" && path === "/git/commits") {
      if (!s.trees.has(body.tree) || !body.parents.every((p: string) => s.commits.has(p))) return json(422, { message: "Validation Failed" });
      return json(201, { sha: await s.putCommit(body.tree, body.parents, body.message, body.author) });
    }
    if (method === "PATCH" && (m = /^\/git\/refs\/heads\/(.+)$/.exec(path))) return this.patchRef(m[1], body);
    if (method === "GET" && (m = /^\/compare\/([0-9a-f]+)\.\.\.([0-9a-f]+)$/.exec(path))) return this.compare(m[1], m[2]);
    if (method === "GET" && path === "/commits") return this.listCommits(q);
    return json(404, { message: "Not Found" });
  }

  private async contents(dir: string, ref: string): Promise<Response> {
    const commit = this.store.resolve(ref);
    if (!commit) return json(404, { message: "No commit found for the ref" });
    const flat = this.store.flatAt(commit);
    const file = flat.get(dir);
    if (file) return json(200, { type: "file", path: dir, sha: file.sha, content: "" });
    const kids = await this.store.children(flat, dir);
    if (!kids) return json(404, { message: "Not Found" });
    const type = (mode: string) => (mode === "040000" ? "dir" : mode === "120000" ? "symlink" : mode === "160000" ? "submodule" : "file");
    return json(200, kids.slice(0, this.contentsCap).map((k) => ({ name: k.name, path: dir ? `${dir}/${k.name}` : k.name, sha: k.sha, size: k.size ?? 0, type: type(k.mode) })));
  }

  private async getTree(sha: string, recursive: boolean): Promise<Response> {
    const flat = this.store.trees.get(sha) ?? this.store.dirTrees.get(sha);
    if (!flat) return json(404, { message: "Not Found" });
    const entries: { path: string; mode: string; type: string; sha: string; size?: number }[] = [];
    const kinds = (mode: string) => (mode === "040000" ? "tree" : mode === "160000" ? "commit" : "blob");
    if (recursive) {
      const dirs = new Set<string>();
      for (const p of flat.keys()) for (let i = p.indexOf("/"); i > 0; i = p.indexOf("/", i + 1)) dirs.add(p.slice(0, i));
      for (const d of dirs) entries.push({ path: d, mode: "040000", type: "tree", sha: "d".repeat(40) });
      for (const [p, e] of flat) entries.push({ path: p, mode: e.mode, type: kinds(e.mode), sha: e.sha, size: this.store.blobs.get(e.sha)?.length });
    } else {
      for (const k of (await this.store.children(flat, ""))!) entries.push({ path: k.name, mode: k.mode, type: kinds(k.mode), sha: k.sha, size: k.size });
    }
    return json(200, { sha, tree: entries, truncated: false });
  }

  private async createTree(body: any): Promise<Response> {
    const base = this.store.trees.get(body.base_tree);
    if (!base) return json(422, { message: "Invalid tree info" });
    const flat: Flat = new Map(base);
    for (const item of body.tree) {
      if (item.sha === null) {
        if (!flat.delete(item.path)) return json(422, { message: "GitHub could not delete the file" });
      } else flat.set(item.path, { mode: item.mode, sha: item.sha });
    }
    return json(201, { sha: await this.store.putTree(flat) });
  }

  private async patchRef(branch: string, body: any): Promise<Response> {
    this.patches.push({ sha: body.sha, force: body.force });
    const hook = this.beforePatch;
    this.beforePatch = null;
    if (hook) await hook();
    if (this.protectedBranches.has(branch)) return json(422, { message: `Protected branch update failed for refs/heads/${branch}.` });
    const current = this.store.refs.get(branch);
    if (!current) return json(422, { message: "Reference does not exist" });
    if (body.force !== true && !this.store.isAncestor(current, body.sha)) return json(422, { message: "Update is not a fast forward" });
    this.store.refs.set(branch, body.sha);
    return json(200, { ref: `refs/heads/${branch}`, object: { sha: body.sha } });
  }

  private compare(a: string, b: string): Response {
    const s = this.store;
    if (!s.commits.has(a) || !s.commits.has(b)) return json(404, { message: "Not Found" });
    const status = a === b ? "identical" : s.isAncestor(a, b) ? "ahead" : s.isAncestor(b, a) ? "behind" : "diverged";
    const fa = s.flatAt(a), fb = s.flatAt(b);
    const files: { filename: string; status: string; sha: string | null }[] = [];
    for (const [p, e] of fb) if (fa.get(p)?.sha !== e.sha) files.push({ filename: p, status: fa.has(p) ? "modified" : "added", sha: e.sha });
    for (const [p, e] of fa) if (!fb.has(p)) files.push({ filename: p, status: "removed", sha: e.sha });
    return json(200, { status, files: files.slice(0, this.compareCap) });
  }

  private listCommits(q: URLSearchParams): Response {
    const s = this.store;
    const path = q.get("path") ?? "";
    const per = Number(q.get("per_page") ?? 30), page = Number(q.get("page") ?? 1);
    const all: string[] = [];
    for (let c = s.resolve(q.get("sha") ?? this.defaultBranch); c; c = s.commits.get(c)!.parents[0]) {
      const parent = s.commits.get(c)!.parents[0];
      if (s.flatAt(c).get(path)?.sha !== (parent ? s.flatAt(parent).get(path)?.sha : undefined)) all.push(c);
    }
    const items = all.slice((page - 1) * per, page * per).map((sha) => ({ sha, commit: { message: s.commits.get(sha)!.message, author: s.commits.get(sha)!.author } }));
    const headers: Record<string, string> = {};
    if (page * per < all.length) headers.link = `<${this.base}/commits?path=${encodeURIComponent(path)}&per_page=${per}&page=${page + 1}>; rel="next", <${this.base}/commits?page=1>; rel="first"`;
    return json(200, items, headers);
  }
}
