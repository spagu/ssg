// The site model: what editor-ui knows about the site, read from the branch's
// own configuration (site-config.ts, site-keys.ts) — nothing about the site
// is repeated in the Worker's variables. Two variables override it for a
// configuration the reader cannot follow: EDITOR_CONTENT_DIRS (comma-separated
// folders) and EDITOR_MEDIA_DIR.
//
// Cached per isolate, keyed by the branch's head commit: one request to the
// repository host (the head) when nothing changed, the files again only after
// a new commit. Overrides are applied on every call, so a changed variable
// never meets a stale cache.

import { normalizePath } from "../portable/path-policy";
import type { RepoConfig } from "../portable/repo-config";
import type { RepoProvider } from "../portable/repo-provider";
import type { Env } from "./http";
import { OpError } from "../portable/ops";
import { readSiteConfig } from "./site-config";
import type { SiteConfig } from "./site-config";
import { ShapeError, siteKeys } from "./site-keys";
import type { SiteKeys, SiteProblem } from "./site-keys";

export type { Section, Shortcode, Mddb, MddbSource, SiteProblem } from "./site-keys";

export interface SiteModel extends SiteKeys {
  head: string; // the commit the model was read at
  branch: string;
  configFile: string | null; // the file read, null when none was usable
  overlay: boolean; // .ssg.designer.json was merged
  overrides: { contentDirs: boolean; mediaDir: boolean };
}

export type SiteErrorCode = "site_config_unreadable" | "no_content_dirs" | "bad_override";
export type SiteState =
  | { ok: true; model: SiteModel }
  | { ok: false; head: string; branch: string; error: SiteErrorCode; detail: string; hint: string };

export const OVERRIDE_HINT =
  "set EDITOR_CONTENT_DIRS (comma-separated repository folders, e.g. \"content/blog, docs\") and EDITOR_MEDIA_DIR " +
  "in wrangler.toml [vars], then deploy";

// Parsed: the configuration at one head, before overrides.
type Parsed = { ok: true; file: string | null; overlay: boolean; keys: SiteKeys } | { ok: false; file: string | null; detail: string };

const cache = new Map<string, { head: string; parsed: Parsed }>();

// clearSiteCache empties the per-isolate cache (tests).
export const clearSiteCache = (): void => cache.clear();

function parsedFrom(config: SiteConfig): Parsed {
  if (!config.ok) return config;
  try {
    return { ok: true, file: config.file, overlay: config.overlay, keys: siteKeys(config.raw) };
  } catch (err) {
    if (!(err instanceof ShapeError)) throw err;
    return { ok: false, file: config.file, detail: `${config.file}: ${err.message}` };
  }
}

// folders reads an override variable: comma-separated repository folders.
function folders(raw: string | undefined, name: string): string[] | { bad: string } | null {
  const parts = (raw ?? "").split(",").map((s) => s.trim()).filter(Boolean);
  if (!parts.length) return null;
  const out: string[] = [];
  for (const p of parts) {
    const v = normalizePath(p.replace(/\/+$/, ""));
    if (!v.ok) return { bad: `${name}: an entry is not a folder inside the repository (${v.reason})` };
    out.push(v.path);
  }
  return out;
}

const EMPTY: SiteKeys = {
  contentRoots: [], sections: [], mediaDir: null, staticDir: null, languages: [], defaultLanguage: null,
  languageSections: {}, shortcodes: [], mddb: null, problems: [],
};

// compose applies the overrides to a parsed configuration.
export function compose(parsed: Parsed, env: Env, head: string, branch: string): SiteState {
  const fail = (error: SiteErrorCode, detail: string): SiteState => ({ ok: false, head, branch, error, detail, hint: OVERRIDE_HINT });
  const dirs = folders(env.EDITOR_CONTENT_DIRS, "EDITOR_CONTENT_DIRS");
  const media = folders(env.EDITOR_MEDIA_DIR, "EDITOR_MEDIA_DIR");
  if (dirs && "bad" in dirs) return fail("bad_override", dirs.bad);
  if (media && "bad" in media) return fail("bad_override", media.bad);
  if (media && media.length > 1) return fail("bad_override", "EDITOR_MEDIA_DIR: one folder only");
  if (!parsed.ok && !dirs) return fail("site_config_unreadable", parsed.detail);
  const problems: SiteProblem[] = parsed.ok ? [...parsed.keys.problems] : [{ code: "site_config_unreadable", detail: parsed.detail }];
  const keys = parsed.ok ? parsed.keys : EMPTY;
  const model: SiteModel = {
    ...keys,
    problems,
    contentRoots: dirs ?? keys.contentRoots,
    mediaDir: media ? media[0] : keys.mediaDir,
    head,
    branch,
    configFile: parsed.ok ? parsed.file : null,
    overlay: parsed.ok && parsed.overlay,
    overrides: { contentDirs: dirs !== null, mediaDir: media !== null },
  };
  if (!model.contentRoots.length) return fail("no_content_dirs", `${parsed.file} names no content folder (source: or content_sources:)`);
  return { ok: true, model };
}

// loadSiteModel reads the model at the tip of `branch`. Errors of the
// repository host (RepoError) are thrown for the caller to answer.
export async function loadSiteModel(provider: RepoProvider, repo: RepoConfig, branch: string, env: Env): Promise<SiteState> {
  const head = await provider.head(branch);
  const key = `${repo.owner}/${repo.repo}#${branch}`;
  let hit = cache.get(key);
  if (hit?.head !== head) {
    hit = { head, parsed: parsedFrom(await readSiteConfig(provider, head)) };
    cache.set(key, hit);
  }
  return compose(hit.parsed, env, head, branch);
}

// requireSiteModel: for an operation that cannot work without the model —
// 503 with the error code, the detail (file and line) and the hint otherwise.
export async function requireSiteModel(provider: RepoProvider, repo: RepoConfig, branch: string, env: Env): Promise<SiteModel> {
  const state = await loadSiteModel(provider, repo, branch, env);
  if (!state.ok) throw new OpError(state.error, 503, state.detail, { hint: state.hint });
  return state.model;
}
