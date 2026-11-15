// The site's typed components (GO-174), read from the `components.json` its
// last build published (docs/COMPONENTS.md, internal/components/render.go
// Manifest). Why the published file and not the component folders in the
// repository: ssg itself wrote it — prop types defaulted, schemas validated,
// a component that failed to load left out — so the editor offers exactly
// what the build accepts, without a second reading of component.yaml in the
// Worker (whose YAML reader is deliberately small); and it is one request
// instead of one per component. The price: a component added on the branch
// is offered after the next deploy publishes it.
//
// The fetch is bounded like the session verifier's JWKS read: the editor's
// own origin only (no credentials in the URL), no redirects followed, a time
// limit and a size limit. The caller's cookie or token is never sent. Any
// failure is an empty section with a status and a hint — the editor works on.

import { rebuildUrl } from "./rebuild";
import type { OpContext } from "./ops";

export const MANIFEST_PATH = "/components.json";
export const MAX_MANIFEST_BYTES = 262_144; // 256 KiB: hundreds of components
export const MANIFEST_TIMEOUT_MS = 5_000;
export const PROP_TYPES = ["string", "number", "bool", "array"] as const;

export interface ComponentProp {
  name: string;
  type: (typeof PROP_TYPES)[number];
  required: boolean;
  default?: string | number | boolean | string[];
  enum?: string[];
  description?: string;
}
export interface ComponentDef { name: string; description: string; example: string; props: ComponentProp[] }
export type ManifestStatus = "ok" | "not_published" | "redirected" | "unavailable" | "too_large" | "invalid" | "bad_url";
export interface ComponentsSource { status: ManifestStatus; url: string | null; httpStatus?: number; hint?: string; items: ComponentDef[] }

const HINTS: Record<Exclude<ManifestStatus, "ok">, string> = {
  not_published: "the site publishes no components.json: ssg writes it when the site has typed components " +
    "(a components/ folder, or components_dir in .ssg.yaml, docs/COMPONENTS.md); add one, publish, and the next deploy lists it",
  redirected: "components.json answered with a redirect, which the editor does not follow",
  unavailable: "components.json could not be read from the site; try again later",
  too_large: `components.json is larger than ${MAX_MANIFEST_BYTES} bytes`,
  invalid: "components.json is not the manifest ssg writes (schema 1)",
  bad_url: "EDITOR_COMPONENTS_URL must be a path on the editor's own site, e.g. /components.json",
};

// ssg's own grammar (internal/components): names a call can address.
const COMPONENT_NAME = /^[a-z0-9_-]{1,64}$/;
const PROP_NAME = /^[A-Za-z_][A-Za-z0-9_-]{0,63}$/;
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const optStr = (v: unknown) => v === undefined || typeof v === "string";
const strList = (v: unknown): v is string[] => Array.isArray(v) && v.every((x) => typeof x === "string");
const scalar = (v: unknown) => ["string", "number", "boolean"].includes(typeof v) || strList(v);

function prop(v: unknown): ComponentProp | null {
  if (!isObj(v) || typeof v.name !== "string" || !PROP_NAME.test(v.name)) return null;
  const type = (v.type ?? "string") as ComponentProp["type"];
  if (!PROP_TYPES.includes(type) || !optStr(v.description)) return null;
  if (v.enum !== undefined && !strList(v.enum)) return null;
  if (v.default !== undefined && v.default !== null && !scalar(v.default)) return null;
  const out: ComponentProp = { name: v.name, type, required: v.required === true };
  if (v.default !== undefined && v.default !== null) out.default = v.default as ComponentProp["default"];
  if (v.enum) out.enum = v.enum as string[];
  if (v.description) out.description = v.description as string;
  return out;
}

function component(v: unknown): ComponentDef | null {
  if (!isObj(v) || typeof v.name !== "string" || !COMPONENT_NAME.test(v.name) || !optStr(v.description) || !optStr(v.example)) return null;
  const props = Array.isArray(v.props) ? v.props.map(prop) : v.props === undefined ? [] : [null];
  if (props.includes(null)) return null;
  return { name: v.name, description: (v.description as string) ?? "", example: (v.example as string) ?? `{{< ${v.name} >}}`, props: props as ComponentProp[] };
}

// parseManifest: the components of a manifest, or null when it is not one —
// one malformed entry makes the whole file untrusted, never half a list.
export function parseManifest(text: string): ComponentDef[] | null {
  let doc: unknown;
  try {
    doc = JSON.parse(text);
  } catch {
    return null;
  }
  if (!isObj(doc) || doc.schema !== 1 || !Array.isArray(doc.components)) return null;
  const items = doc.components.map(component);
  return items.includes(null) ? null : (items as ComponentDef[]).sort((a, b) => a.name.localeCompare(b.name));
}

// readBounded: the body as text, or null past `max` bytes (the rest unread).
async function readBounded(res: Response, max: number): Promise<string | null> {
  if (Number(res.headers.get("content-length") ?? 0) > max) return null;
  const reader = res.body!.getReader();
  const dec = new TextDecoder();
  let [text, size] = ["", 0];
  for (let r = await reader.read(); !r.done; r = await reader.read()) {
    if ((size += r.value.byteLength) > max) {
      await reader.cancel();
      return null;
    }
    text += dec.decode(r.value, { stream: true });
  }
  return text + dec.decode();
}

// readComponents: the published components, or an empty section saying why.
export async function readComponents(ctx: OpContext): Promise<ComponentsSource> {
  const raw = ((ctx.env as { EDITOR_COMPONENTS_URL?: string }).EDITOR_COMPONENTS_URL ?? "").trim() || MANIFEST_PATH;
  const url = rebuildUrl(raw, ctx.url);
  const empty = (status: Exclude<ManifestStatus, "ok">, extra: Partial<ComponentsSource> = {}): ComponentsSource =>
    ({ status, url: url?.href ?? null, hint: HINTS[status], items: [], ...extra });
  if (!url) return empty("bad_url");
  const doFetch = ctx.deps.fetch ?? ((u: string, init?: RequestInit) => fetch(u, init));
  let res: Response;
  try {
    res = await doFetch(url.href, { redirect: "manual", headers: { accept: "application/json" }, signal: AbortSignal.timeout(MANIFEST_TIMEOUT_MS) });
  } catch {
    return empty("unavailable");
  }
  if (res.status === 404) return empty("not_published", { httpStatus: 404 });
  if (res.status >= 300 && res.status < 400) return empty("redirected", { httpStatus: res.status });
  if (res.status !== 200 || !res.body) return empty("unavailable", { httpStatus: res.status });
  const text = await readBounded(res, MAX_MANIFEST_BYTES).catch(() => undefined);
  if (text === undefined) return empty("unavailable");
  if (text === null) return empty("too_large");
  const items = parseManifest(text);
  return items ? { status: "ok", url: url.href, items } : empty("invalid");
}
