// The site's configuration files on the branch, as ssg itself finds and
// merges them: the config file in ssg's search order (internal/config
// FindConfigFile), the settings overlay .ssg.designer.json beside it (GO-177),
// and the overlay merge of internal/config/overlay.go. What each worker
// reads out of them is its own (editor-ui's site-config.ts, designer-ui's
// config-read.ts). Shared by the panel workers (GO-185).

import { RepoError } from "./repo-provider";
import type { RepoProvider } from "./repo-provider";

// CONFIG_FILES: ssg's own search order (internal/config/config.go).
export const CONFIG_FILES = [".ssg.yaml", ".ssg.yml", ".ssg.toml", ".ssg.json", "ssg.yaml", "ssg.yml", "ssg.toml", "ssg.json"];
// OVERLAY_FILE: the settings overlay designer-ui writes and ssg merges.
export const OVERLAY_FILE = ".ssg.designer.json";

export type Doc = Record<string, unknown>;

export const isObject = (v: unknown): v is Doc => typeof v === "object" && v !== null && !Array.isArray(v);

// utf8: the bytes as text, or undefined when they are not UTF-8.
export function utf8(bytes: Uint8Array): string | undefined {
  try {
    return new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(bytes);
  } catch {
    return undefined;
  }
}

// readText: the file's text and version at `ref`; null when it does not
// exist; text undefined when it is not UTF-8. Other host failures are thrown.
export async function readText(provider: RepoProvider, ref: string, path: string): Promise<{ text: string | undefined; blobId: string } | null> {
  let file;
  try {
    file = await provider.read(ref, path);
  } catch (err) {
    if (err instanceof RepoError && err.code === "not_found") return null;
    throw err;
  }
  return { text: utf8(file.bytes), blobId: file.blobId };
}

// jsonObject parses a JSON object, or null. A parse error quotes the text
// around it, which may hold a secret, so it never leaves this function.
export function jsonObject(body: string): Doc | null {
  try {
    const doc: unknown = JSON.parse(body);
    return isObject(doc) ? doc : null;
  } catch {
    return null;
  }
}

// merge applies an overlay as ssg does (internal/config/overlay.go): mappings
// merge key by key, anything else replaces, null removes the key. Keys are
// stored as own data (GO-183): the overlay is JSON, whose "__proto__" key an
// assignment would turn into the merged mapping's prototype.
export function merge<T extends Doc>(base: T, overlay: Doc): T {
  const out: Doc = { ...base };
  for (const [k, v] of Object.entries(overlay)) {
    if (v === null) delete out[k];
    else {
      const here = Object.hasOwn(out, k) ? out[k] : undefined;
      Object.defineProperty(out, k, { value: isObject(v) && isObject(here) ? merge(here, v) : v, enumerable: true, writable: true, configurable: true });
    }
  }
  return out as T;
}
