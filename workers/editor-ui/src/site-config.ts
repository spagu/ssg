// Reading the site's configuration from the branch: the config file ssg itself
// would pick (internal/config FindConfigFile's order), the settings overlay
// .ssg.designer.json beside it (GO-177: merged over the file, mappings key by
// key, anything else replaced, null removes), and only the top-level keys the
// editor needs. Never a guess: a file the reader cannot follow is reported
// with its line, and the site model points to the override variables. The
// files, their order and the merge are portable/config-files.ts.

import { CONFIG_FILES, OVERLAY_FILE, jsonObject, merge, readText } from "../portable/config-files";
import type { RepoProvider } from "../portable/repo-provider";
import { YamlUnsupported, readKeys } from "../portable/yaml-lite";
import type { YamlValue } from "../portable/yaml-lite";

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

// pick keeps the WANTED keys of a parsed JSON document (JSON values are YAML
// values, so the cast only names them).
const pick = (doc: Record<string, unknown>): Raw => Object.fromEntries(Object.entries(doc).filter(([k]) => WANTED.has(k))) as Raw;

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
    if (body.text === undefined) return { ok: false, file, detail: `${file} is not UTF-8 text` };
    let raw: Raw;
    try {
      raw = parse(file, body.text);
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
  if (body.text === undefined) return { ok: false, file, detail: `${OVERLAY_FILE} is not UTF-8 text` };
  const doc = jsonObject(body.text);
  if (!doc) return { ok: false, file, detail: `${OVERLAY_FILE} is not a JSON object` };
  return { ok: true, file, overlay: true, raw: merge(raw, pick(doc)) };
}
