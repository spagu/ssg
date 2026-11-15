// Reading the site's configuration from the branch: the file ssg itself would
// pick (internal/config FindConfigFile's order), every top-level key of it the
// minimal YAML reader can follow, and the overlay .ssg.designer.json beside
// it. The worker reads .ssg.yaml, never writes it (README, decision 7).
//
// Why a reader in the Worker (GO-178, "Odczyt wartości z .ssg.yaml"): ssg
// publishes no effective configuration into the repository, and a build
// artefact would lag the branch the panel edits. So the reader of editor-ui
// (portable/yaml-lite.ts, a byte copy) reads key by key: a key it cannot follow is
// reported as "set in the file, value not read" with the line, never guessed,
// and the other keys still read. The files, their order, reading them and
// the overlay merge are portable/config-files.ts, shared with editor-ui.

import { CONFIG_FILES, OVERLAY_FILE, isObject, jsonObject, merge, readText } from "../portable/config-files";
import type { Doc } from "../portable/config-files";
import type { RepoProvider } from "../portable/repo-provider";
import { readKeys } from "../portable/yaml-lite";
import type { YamlUnsupported } from "../portable/yaml-lite";

export type Format = "yaml" | "json" | "toml";

export interface ConfigState {
  head: string; // the commit the state was read at
  file: string | null; // the config file found, null when there is none
  format: Format | null;
  values: Doc; // the top-level keys read from the file
  unread: Record<string, string>; // top-level keys present but not read → why
  fileProblem: string | null; // the file as a whole could not be read
  include: boolean; // the file uses include: (included files are not followed)
  overlay: Doc | null; // the overlay as written, null when absent or unreadable
  overlayBlobId: string | null; // its version (null: no overlay file)
  overlayProblem: string | null;
}

// TOP_KEY: a top-level "key:" at column 0 — which keys a file sets, even when
// their values are beyond the reader.
const TOP_KEY = /^([A-Za-z_][A-Za-z0-9_.-]*)[ \t]*:(?:[ \t]|$)/;
export const presentKeys = (text: string): string[] =>
  [...new Set(text.replace(/^\uFEFF/, "").split(/\r?\n/).flatMap((l) => TOP_KEY.exec(l)?.[1] ?? []))];

// readYaml reads every top-level key; when the whole file is beyond the
// reader, key by key, so one odd key costs only itself.
export function readYaml(text: string): { values: Doc; unread: Record<string, string> } {
  const keys = presentKeys(text);
  try {
    return { values: readKeys(text, new Set(keys)), unread: {} };
  } catch {
    // Something in the file is beyond the reader: find out which keys, one by one.
  }
  const values: Doc = {};
  const unread: Record<string, string> = {};
  for (const k of keys) {
    try {
      Object.assign(values, readKeys(text, new Set([k])));
    } catch (err) {
      const e = err as YamlUnsupported; // the reader throws nothing else
      unread[k] = `line ${e.line} uses ${e.reason}`;
    }
  }
  return { values, unread };
}

// readFile reads the config file found at `ref` (or says there is none).
async function readFile(provider: RepoProvider, ref: string): Promise<Omit<ConfigState, "head" | "overlay" | "overlayBlobId" | "overlayProblem">> {
  const none = { values: {}, unread: {}, include: false };
  for (const file of CONFIG_FILES) {
    const body = await readText(provider, ref, file);
    if (body === null) continue;
    const format: Format = file.endsWith(".toml") ? "toml" : file.endsWith(".json") ? "json" : "yaml";
    if (body.text === undefined) return { ...none, file, format, fileProblem: `${file} is not UTF-8 text` };
    if (format === "toml") return { ...none, file, format, fileProblem: `${file} is TOML, which this worker does not read` };
    if (format === "json") {
      const doc = jsonObject(body.text);
      if (!doc) return { ...none, file, format, fileProblem: `${file} is not a JSON object` };
      return { file, format, values: doc, unread: {}, fileProblem: null, include: doc.include !== undefined && doc.include !== null };
    }
    const { values, unread } = readYaml(body.text);
    return { file, format, values, unread, fileProblem: null, include: "include" in values || "include" in unread };
  }
  return { ...none, file: null, format: null, fileProblem: `no configuration file (${CONFIG_FILES.join(", ")}) at the repository's root` };
}

// readOverlay reads .ssg.designer.json at `ref`.
async function readOverlay(provider: RepoProvider, ref: string): Promise<Pick<ConfigState, "overlay" | "overlayBlobId" | "overlayProblem">> {
  const body = await readText(provider, ref, OVERLAY_FILE);
  if (body === null) return { overlay: null, overlayBlobId: null, overlayProblem: null };
  const doc = body.text === undefined ? null : jsonObject(body.text);
  if (!doc) return { overlay: null, overlayBlobId: body.blobId, overlayProblem: `${OVERLAY_FILE} is not a JSON object` };
  return { overlay: doc, overlayBlobId: body.blobId, overlayProblem: null };
}

// Cached per isolate, keyed by repository and branch, valid for one head: one
// request to the host (the head) when nothing changed.
const cache = new Map<string, ConfigState>();
export const clearConfigCache = (): void => cache.clear();

// readConfig: the configuration at the tip of `branch` of `repoKey`'s repository.
export async function readConfig(provider: RepoProvider, repoKey: string, branch: string): Promise<ConfigState> {
  const head = await provider.head(branch);
  const key = `${repoKey}#${branch}`;
  const hit = cache.get(key);
  if (hit?.head === head) return hit;
  const state: ConfigState = { head, ...(await readFile(provider, head)), ...(await readOverlay(provider, head)) };
  cache.set(key, state);
  return state;
}

// lookup: the value at a path of keys, and whether it is there at all.
export function lookup(doc: Doc | null, segments: readonly string[]): { has: boolean; value: unknown } {
  let at: unknown = doc;
  for (const s of segments) {
    if (!isObject(at) || !Object.hasOwn(at, s)) return { has: false, value: undefined };
    at = at[s];
  }
  return { has: true, value: at };
}

// effective: what ssg would load, as far as the worker could read the file.
export const effective = (s: ConfigState): Doc => merge(s.values, s.overlay ?? {});
