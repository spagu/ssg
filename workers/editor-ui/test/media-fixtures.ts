// Images for the media suites: real signatures, canonical paths as the panel
// computes them, binary files seeded straight into the fake store.
import { BASE, ORIGIN, makeEnv, request, run } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";
import { mediaPathFor, sha256Hex, sniffImage } from "../src/media-name.ts";
import { PERM } from "../src/permissions.ts";

export const MEDIA = "content/blog/media";
export const WHEN = Date.UTC(2026, 10, 14);
export const AUTHOR = [PERM.read, PERM.publish, PERM.media];

export const bytesOf = (...parts: (number[] | string)[]): Uint8Array<ArrayBuffer> =>
  new Uint8Array(parts.flatMap((p) => (typeof p === "string" ? [...new TextEncoder().encode(p)] : p)));
const PNG_SIG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
export const png = (tail: string) => bytesOf(PNG_SIG, tail);
export const jpeg = (tail: string) => bytesOf([0xff, 0xd8, 0xff, 0xe0], tail);
export const b64 = (b: Uint8Array) => Buffer.from(b).toString("base64");

// image: the change for `bytes` at its canonical path (the type from the bytes, png when they are none).
export async function image(bytes: Uint8Array, name = "Cat.png") {
  const path = mediaPathFor(MEDIA, WHEN, name, await sha256Hex(bytes), sniffImage(bytes) ?? "png");
  return { op: "put", path, version: null, base64: b64(bytes) };
}

// seedBytes commits binary files on main, as someone else would.
export async function seedBytes(gh: FakeGitHub, files: Record<string, Uint8Array<ArrayBuffer>>): Promise<void> {
  const s = gh.store;
  const parent = s.refs.get("main")!;
  const flat = new Map(s.flatAt(parent));
  for (const [path, b] of Object.entries(files)) flat.set(path, { mode: "100644", sha: await s.putBlob(b) });
  s.refs.set("main", await s.putCommit(await s.putTree(flat), [parent], "seed bytes"));
}

export async function publish(gh: FakeGitHub, body: unknown, perms: string[] = AUTHOR, env = makeEnv()): Promise<Response> {
  const r = await request(BASE + "/api/changes", perms, {
    method: "POST", body: JSON.stringify(body), headers: { origin: ORIGIN, "content-type": "application/json" },
  });
  return run(r, env, gh);
}

export async function get(gh: FakeGitHub, path: string, env = makeEnv(), perms: string[] = [PERM.read]): Promise<Response> {
  return run(await request(BASE + path, perms), env, gh);
}

export const headOf = (gh: FakeGitHub) => gh.store.commits.get(gh.store.refs.get("main")!)!;
export const bytesAt = (gh: FakeGitHub, path: string) => gh.store.blobs.get(gh.store.blobIdAt("main", path)!);
