// In-process git object store behind the fake GitHub API (fake-github.ts). Real git blob ids (SHA-1 of
// "blob <len>\0<bytes>"), so ids the provider receives behave like GitHub's; trees are kept as flat
// path → entry maps (one per root tree), commits as {tree, parents}. WebCrypto only, no node: imports, so
// the GO-168 measurement Worker can bundle it too.
export interface Entry { mode: string; sha: string }
export type Flat = Map<string, Entry>;
export interface CommitObj { tree: string; parents: string[]; message: string; author: { name: string; email: string; date: string } }

const enc = new TextEncoder();
const hex = (b: ArrayBuffer) => [...new Uint8Array(b)].map((x) => x.toString(16).padStart(2, "0")).join("");
export const sha1 = async (bytes: Uint8Array<ArrayBuffer>) => hex(await crypto.subtle.digest("SHA-1", bytes));

export class GitStore {
  blobs = new Map<string, Uint8Array<ArrayBuffer>>();
  trees = new Map<string, Flat>(); // root trees
  dirTrees = new Map<string, Flat>(); // sub-directory trees, paths relative to the directory
  commits = new Map<string, CommitObj>();
  refs = new Map<string, string>();
  private clock = 0;

  async putBlob(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
    const head = enc.encode(`blob ${bytes.length}\0`);
    const all = new Uint8Array(head.length + bytes.length);
    all.set(head);
    all.set(bytes, head.length);
    const sha = await sha1(all);
    this.blobs.set(sha, bytes);
    return sha;
  }

  async putTree(flat: Flat, into = this.trees): Promise<string> {
    const sorted = [...flat.entries()].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
    const sha = await sha1(enc.encode("tree " + JSON.stringify(sorted)));
    into.set(sha, flat);
    return sha;
  }

  async putCommit(tree: string, parents: string[], message: string, author = { name: "Seed", email: "seed@example.com" }): Promise<string> {
    const obj = { tree, parents, message, author: { ...author, date: new Date(Date.UTC(2026, 0, 1) + this.clock++ * 1000).toISOString() } };
    const sha = await sha1(enc.encode("commit " + JSON.stringify(obj)));
    this.commits.set(sha, obj);
    return sha;
  }

  // resolve: a branch name or commit id → commit id (or undefined).
  resolve(ref: string): string | undefined {
    return this.refs.get(ref) ?? (this.commits.has(ref) ? ref : undefined);
  }
  flatAt(commit: string): Flat { return this.trees.get(this.commits.get(commit)!.tree)!; }

  isAncestor(a: string, b: string): boolean {
    const seen = new Set<string>();
    const stack = [b];
    while (stack.length) {
      const c = stack.pop()!;
      if (c === a) return true;
      if (seen.has(c)) continue;
      seen.add(c);
      stack.push(...(this.commits.get(c)?.parents ?? []));
    }
    return false;
  }

  // children: the direct entries of `dir` in `flat` (sub-directories as trees, registered in dirTrees).
  async children(flat: Flat, dir: string): Promise<{ name: string; mode: string; sha: string; size?: number }[] | null> {
    const prefix = dir ? dir + "/" : "";
    const files: { name: string; mode: string; sha: string; size?: number }[] = [];
    const subdirs = new Set<string>();
    for (const [path, e] of flat) {
      if (!path.startsWith(prefix)) continue;
      const rest = path.slice(prefix.length);
      if (rest.includes("/")) subdirs.add(rest.slice(0, rest.indexOf("/")));
      else files.push({ name: rest, mode: e.mode, sha: e.sha, size: this.blobs.get(e.sha)?.length ?? 0 });
    }
    if (dir && !files.length && !subdirs.size) return null;
    for (const name of [...subdirs].sort()) {
      const sub: Flat = new Map();
      for (const [path, e] of flat) if (path.startsWith(prefix + name + "/")) sub.set(path.slice(prefix.length + name.length + 1), e);
      files.push({ name, mode: "040000", sha: await this.putTree(sub, this.dirTrees) });
    }
    return files;
  }

  // commitFiles: a direct commit on `branch` (the "someone else" of the race tests, and seeding).
  // Values: text, null to delete, {symlink: target} for a mode-120000 entry.
  async commitFiles(branch: string, files: Record<string, string | null | { symlink: string } | { submodule: string }>, message = "seed"): Promise<string> {
    const parent = this.refs.get(branch);
    const flat: Flat = new Map(parent ? this.flatAt(parent) : []);
    for (const [path, v] of Object.entries(files)) {
      if (v === null) flat.delete(path);
      else if (typeof v === "string") flat.set(path, { mode: "100644", sha: await this.putBlob(enc.encode(v)) });
      else if ("symlink" in v) flat.set(path, { mode: "120000", sha: await this.putBlob(enc.encode(v.symlink)) });
      else flat.set(path, { mode: "160000", sha: v.submodule });
    }
    const sha = await this.putCommit(await this.putTree(flat), parent ? [parent] : [], message);
    this.refs.set(branch, sha);
    return sha;
  }

  blobIdAt(branch: string, path: string): string | null {
    return this.flatAt(this.refs.get(branch)!).get(path)?.sha ?? null;
  }
  text(sha: string): string { return new TextDecoder().decode(this.blobs.get(sha)); }
}
