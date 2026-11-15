// Reading the site's configuration from the branch: the config file ssg itself
// would pick (internal/config FindConfigFile's order), the settings overlay
// .ssg.designer.json beside it (GO-177: merged over the file, mappings key by
// key, anything else replaced, null removes), and only the top-level keys the
// editor needs. Never a guess: a file the reader cannot follow is reported
// with its line, and the site model points to the override variables.

import { RepoError } from "../portable/repo-provider";
import type { RepoProvider } from "../portable/repo-provider";
import { YamlUnsupported, readKeys } from "./yaml-lite";
import type { YamlValue } from "./yaml-lite";

// CONFIG_FILES: ssg's own search order (internal/config/config.go).
export const CONFIG_FILES = [".ssg.yaml", ".ssg.yml", ".ssg.toml", ".ssg.json", "ssg.yaml", "ssg.yml", "ssg.toml", "ssg.json"];
export const OVERLAY_FILE = ".ssg.designer.json";

// WANTED: the keys the site model reads (internal/config/config.go names).
// `include` is read only to notice it: included files are not followed.
export const WANTED: ReadonlySet<string> = new Set([
  "content_dir", "source", "static_dir", "pages_path", "posts_path", "content_sources",
  "languages", "default_language", "language_sections", "shortcodes", "mddb", "include",
  "slug_transliterate", "mermaid", "math", "shortcode_brackets",
]);

export type Raw = Record<string, YamlValue>;
// SiteConfig: what the branch says, or why it cannot be read (detail names the
// file and line, never a value).
export type SiteConfig =
  | { ok: true; file: string | null; overlay: boolean; raw: Raw }
  | { ok: false; file: string | null; detail: string };

const isMap = (v: unknown): v is Record<string, YamlValue> => typeof v === "object" && v !== null && !Array.isArray(v);

// readText: the file's text at `ref`; null when it does not exist, undefined
// when it is not UTF-8. Any other failure of the host is thrown.
async function readText(provider: RepoProvider, ref: string, path: string): Promise<string | null | undefined> {
  let bytes: Uint8Array;
  try {
    bytes = (await provider.read(ref, path)).bytes;
  } catch (err) {
    if (err instanceof RepoError && err.code === "not_found") return null;
    throw err;
  }
  try {
    return new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(bytes);
  } catch {
    return undefined;
  }
}

// jsonObject parses a JSON object, or null. A parse error quotes the text
// around it, which may hold a secret, so it never leaves this function.
function jsonObject(body: string): Record<string, YamlValue> | null {
  try {
    const doc: unknown = JSON.parse(body);
    return isMap(doc) ? doc : null;
  } catch {
    return null;
  }
}

// pick keeps the WANTED keys of a parsed JSON document.
const pick = (doc: Record<string, YamlValue>): Raw => Object.fromEntries(Object.entries(doc).filter(([k]) => WANTED.has(k)));

// merge applies the overlay as ssg does (internal/config/overlay.go). Keys
// are stored as own data (GO-183): the overlay is JSON, whose "__proto__" key
// an assignment would turn into the merged mapping's prototype.
export function merge(base: Raw, overlay: Raw): Raw {
  const out: Raw = { ...base };
  for (const [k, v] of Object.entries(overlay)) {
    if (v === null) delete out[k];
    else {
      const here = Object.hasOwn(out, k) ? out[k] : undefined;
      Object.defineProperty(out, k, { value: isMap(v) && isMap(here) ? merge(here, v) : v, enumerable: true, writable: true, configurable: true });
    }
  }
  return out;
}

// parse reads one config file's WANTED keys by its extension.
export function parse(file: string, body: string): Raw {
  if (file.endsWith(".toml")) throw new Error("it is TOML, which this worker does not read");
  if (!file.endsWith(".json")) return readKeys(body, WANTED);
  const doc = jsonObject(body);
  if (!doc) throw new Error("it is not a JSON object");
  return pick(doc);
}

// readSiteConfig finds and reads the configuration at `ref` (a commit id).
export async function readSiteConfig(provider: RepoProvider, ref: string): Promise<SiteConfig> {
  for (const file of CONFIG_FILES) {
    const body = await readText(provider, ref, file);
    if (body === null) continue;
    if (body === undefined) return { ok: false, file, detail: `${file} is not UTF-8 text` };
    let raw: Raw;
    try {
      raw = parse(file, body);
    } catch (err) {
      return { ok: false, file, detail: `${file}: ${err instanceof YamlUnsupported ? `line ${err.line} uses ${err.reason}` : (err as Error).message}` };
    }
    if (raw.include !== undefined && raw.include !== null) return { ok: false, file, detail: `${file} uses include:, and included files are not followed` };
    return withOverlay(provider, ref, file, raw);
  }
  return { ok: false, file: null, detail: `no configuration file (${CONFIG_FILES.join(", ")}) at the repository's root` };
}

// withOverlay merges .ssg.designer.json when it exists.
async function withOverlay(provider: RepoProvider, ref: string, file: string, raw: Raw): Promise<SiteConfig> {
  const body = await readText(provider, ref, OVERLAY_FILE);
  if (body === null) return { ok: true, file, overlay: false, raw };
  if (body === undefined) return { ok: false, file, detail: `${OVERLAY_FILE} is not UTF-8 text` };
  const doc = jsonObject(body);
  if (!doc) return { ok: false, file, detail: `${OVERLAY_FILE} is not a JSON object` };
  return { ok: true, file, overlay: true, raw: merge(raw, pick(doc)) };
}
