// Setup for the MDDB source's tests: the fake MDDB (fake-mddb.ts), the fake
// GitHub of fixtures.ts, and a stand-in for a rebuild endpoint on the site's
// own host — one fetch that sends each request to its host. No network.
import { FakeMddb, MDDB_HOST, MDDB_KEY } from "./fake-mddb.ts";
import type { FakeGitHub } from "./fake-github.ts";
import { AUTH_VARS, BASE, GIT_VARS, ORIGIN, SITE_YAML, assets, folderHistory, request, site } from "./fixtures.ts";
import { handleFetch } from "../src/app.ts";
import { clearListingCache } from "../src/content-list.ts";
import { clearMetaCache } from "../src/content-meta.ts";

export const MDDB_VARS = { EDITOR_MDDB_URL: `https://${MDDB_HOST}`, EDITOR_MDDB_API_KEY: MDDB_KEY };
export const REBUILD_KEY = "rebuild-secret-never-shown";

// MDDB_SITE_YAML: the seeded site, built from two MDDB sources.
export const MDDB_SITE_YAML = SITE_YAML.replace(
  "mddb:\n  enabled: false\n",
  "mddb:\n  enabled: true\n  lang: en\n  sources:\n    - collection: blog\n      section: blog\n    - collection: kb\n      type: page\n      filter:\n        visibility: [public]\n",
);

export const mddbOnlyEnv = (over: Record<string, unknown> = {}) => ({ ...AUTH_VARS, ASSETS: assets, ...MDDB_VARS, EDITOR_MDDB_COLLECTION: "blog", ...over });
export const bothEnv = (over: Record<string, unknown> = {}) => ({ ...AUTH_VARS, ...GIT_VARS, ASSETS: assets, ...MDDB_VARS, ...over });

export interface World { db: FakeMddb; gh: FakeGitHub | null; rebuilds: { url: string; headers: Headers }[]; rebuildStatus: number | "throw" }

// world: a fresh fake MDDB, and the fake GitHub seeded with MDDB_SITE_YAML (git: false = none).
export async function world(opts: { git?: boolean; files?: Record<string, string | null> } = {}): Promise<World> {
  clearListingCache();
  clearMetaCache();
  const gh = opts.git === false ? null : await site({ ".ssg.yaml": MDDB_SITE_YAML, ...opts.files });
  return { db: new FakeMddb(), gh, rebuilds: [], rebuildStatus: 202 };
}

export function worldFetch(w: World) {
  return async (url: string, init: RequestInit = {}): Promise<Response> => {
    const host = new URL(url).host;
    if (host === MDDB_HOST) return w.db.fetch(url, init);
    if (host === new URL(ORIGIN).host) {
      w.rebuilds.push({ url, headers: new Headers(init.headers) });
      if (w.rebuildStatus === "throw") throw new Error("down");
      return new Response(null, { status: w.rebuildStatus });
    }
    return folderHistory(w.gh!)(url, init); // a world without a repository makes no such request
  };
}

type Init = RequestInit & { kind?: "user" | "service" };
// send runs one request through the whole pipeline in the world.
export async function send(w: World, env: Record<string, unknown>, path: string, perms: string[] | null, init: Init = {}): Promise<Response> {
  return handleFetch(await request(BASE + path, perms, init), env as never, { fetch: worldFetch(w) });
}

// post: a JSON POST from the panel (same origin).
export const post = (w: World, env: Record<string, unknown>, path: string, perms: string[], body: unknown) =>
  send(w, env, path, perms, { method: "POST", body: JSON.stringify(body), headers: { origin: ORIGIN, "content-type": "application/json" } });

export const seg = encodeURIComponent;
