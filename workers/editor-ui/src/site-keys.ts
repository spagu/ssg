// From the configuration's raw keys to what the editor needs to know about
// the site: where content lives, its sections, the media and static folders,
// languages, shortcodes, MDDB sources. Key names and defaults are ssg's own
// (internal/config/config.go: DefaultConfig, Config; cmd/ssg/mcp.go
// mediaRootsOf for the media folder). Values that are not plain repository
// folders — an absolute path, "..", a $VARIABLE resolved at build time — are
// left out and reported, never followed.

import { normalizePath } from "../portable/path-policy";
import type { Raw } from "./site-config";
import type { YamlValue } from "./yaml-lite";

export interface Section { path: string; type: "page" | "post" }
export interface Shortcode { name: string; type: string | null; template: string | null; title: string | null }
// filter and lang are present only when the configuration sets them (GO-175).
export interface MddbSource { collection: string | null; type: string | null; section: string | null; filter?: Record<string, string[]> }
export interface Mddb { enabled: boolean; collection: string | null; sources: MddbSource[]; lang?: string }
export interface SiteProblem { code: string; detail: string }

// SiteKeys: the part of the site model read from the configuration.
export interface SiteKeys {
  contentRoots: string[]; // folders whose files the editor may list and change
  sections: Section[]; // where new pages and posts go
  mediaDir: string | null; // where pasted images go (null: unknown)
  staticDir: string | null;
  languages: string[];
  defaultLanguage: string | null;
  languageSections: Record<string, string>;
  shortcodes: Shortcode[];
  mddb: Mddb | null;
  problems: SiteProblem[];
  slugTransliterate?: true; // slug_transliterate: true (#426); absent when off
  // What the widget list offers (GO-174); each absent when off.
  mermaid?: true; // mermaid: true — ```mermaid fences become diagrams
  math?: true; // math: true — $$…$$ and ```math are typeset
  shortcodeBrackets?: true; // shortcode_brackets: true — [name] works beside {{name}}
}

// ShapeError: a key holds a value of a kind ssg itself would refuse.
export class ShapeError extends Error {}

const isMap = (v: unknown): v is Record<string, YamlValue> => typeof v === "object" && v !== null && !Array.isArray(v);

// scalar: a string (numbers read as their text), null when absent.
function scalar(v: YamlValue | undefined, key: string): string | null {
  if (v === undefined || v === null) return null;
  if (typeof v === "string" || typeof v === "number") return String(v);
  throw new ShapeError(`${key} is not a single value`);
}
const list = (v: YamlValue | undefined, key: string): YamlValue[] => {
  if (v === undefined || v === null) return [];
  if (!Array.isArray(v)) throw new ShapeError(`${key} is not a list`);
  return v;
};
const map = (v: YamlValue | undefined, key: string): Record<string, YamlValue> => {
  if (v === undefined || v === null) return {};
  if (!isMap(v)) throw new ShapeError(`${key} is not a mapping`);
  return v;
};

// folder: a repository-relative folder, or a problem saying why not.
function folder(raw: string, key: string, problems: SiteProblem[]): string | null {
  if (raw.startsWith("$")) {
    problems.push({ code: "env_reference", detail: `${key} is set from an environment variable at build time` });
    return null;
  }
  const v = normalizePath(raw.replace(/^(?:\.\/)+/, "").replace(/\/+$/, ""));
  if (v.ok) return v.path;
  problems.push({ code: "outside_repository", detail: `${key} is not a folder inside the repository (${v.reason})` });
  return null;
}

const join = (...parts: string[]) => parts.filter(Boolean).join("/");

// siteKeys maps the raw keys; throws ShapeError for a value of the wrong kind.
export function siteKeys(raw: Raw): SiteKeys {
  const problems: SiteProblem[] = [];
  const contentDir = folder(scalar(raw.content_dir, "content_dir") ?? "content", "content_dir", problems);
  const sourceRaw = scalar(raw.source, "source");
  const source = sourceRaw === null ? null : folder(sourceRaw, "source", problems);
  const contentRoots: string[] = [];
  const sections: Section[] = [];
  let mediaDir: string | null = null;
  if (contentDir !== null && source !== null) {
    const root = join(contentDir, source);
    contentRoots.push(root);
    mediaDir = join(root, "media");
    for (const [key, type, def] of [["pages_path", "page", "pages"], ["posts_path", "post", "posts"]] as const) {
      const sub = folder(scalar(raw[key], key) ?? def, key, problems);
      if (sub !== null) sections.push({ path: join(root, sub), type });
    }
  }
  list(raw.content_sources, "content_sources").forEach((entry, i) => {
    const e = map(entry, `content_sources[${i}]`);
    const path = folder(scalar(e.path, `content_sources[${i}].path`) ?? "", `content_sources[${i}].path`, problems);
    if (path === null) return;
    contentRoots.push(path);
    sections.push({ path, type: scalar(e.type, `content_sources[${i}].type`) === "post" ? "post" : "page" });
  });
  const staticRaw = scalar(raw.static_dir, "static_dir") ?? "static";
  return {
    contentRoots: [...new Set(contentRoots)],
    sections: sections.filter((s, i) => sections.findIndex((t) => t.path === s.path) === i), // first one wins
    mediaDir,
    staticDir: folder(staticRaw, "static_dir", problems),
    languages: languagesOf(raw.languages),
    defaultLanguage: scalar(raw.default_language, "default_language"),
    languageSections: Object.fromEntries(Object.entries(map(raw.language_sections, "language_sections")).map(([k, v]) => [k, scalar(v, `language_sections.${k}`) ?? ""])),
    shortcodes: shortcodesOf(raw.shortcodes),
    mddb: mddbOf(raw.mddb),
    problems,
    ...(raw.slug_transliterate === true ? { slugTransliterate: true as const } : {}),
    ...switches(raw),
  };
}

// switches: the rendering options that decide which built-in blocks the
// widget list offers — only `true` turns one on, as ssg reads a bool.
function switches(raw: Raw): Pick<SiteKeys, "mermaid" | "math" | "shortcodeBrackets"> {
  return {
    ...(raw.mermaid === true ? { mermaid: true as const } : {}),
    ...(raw.math === true ? { math: true as const } : {}),
    ...(raw.shortcode_brackets === true ? { shortcodeBrackets: true as const } : {}),
  };
}

// shortcodesOf: name, type, template and title of each shortcode; one without a
// name cannot be used in content (ssg matches by name), so it is left out.
function shortcodesOf(v: YamlValue | undefined): Shortcode[] {
  return list(v, "shortcodes").flatMap((s, i) => {
    const e = map(s, `shortcodes[${i}]`);
    const at = (k: string) => scalar(e[k], `shortcodes[${i}].${k}`);
    const name = at("name");
    return name ? [{ name, type: at("type"), template: at("template"), title: at("title") }] : [];
  });
}

// languagesOf: ["en", "pl"] or the expanded form [{code: en, …}, …].
function languagesOf(v: YamlValue | undefined): string[] {
  return list(v, "languages").map((l, i) => (isMap(l) ? scalar(l.code, `languages[${i}].code`) : scalar(l, `languages[${i}]`)) ?? "").filter(Boolean);
}

// mddbOf keeps what says where articles come from — never url or api_key.
function mddbOf(v: YamlValue | undefined): Mddb | null {
  if (v === undefined || v === null) return null;
  const m = map(v, "mddb");
  const sources = list(m.sources, "mddb.sources").map((s, i) => {
    const e = map(s, `mddb.sources[${i}]`);
    const at = (k: string) => scalar(e[k], `mddb.sources[${i}].${k}`);
    const filter = Object.entries(map(e.filter, `mddb.sources[${i}].filter`))
      .map(([k, f]) => [k, list(f, `mddb.sources[${i}].filter.${k}`).map((x, j) => scalar(x, `mddb.sources[${i}].filter.${k}[${j}]`) ?? "")]);
    return { collection: at("collection"), type: at("type"), section: at("section"), ...(filter.length ? { filter: Object.fromEntries(filter) } : {}) };
  });
  const lang = scalar(m.lang, "mddb.lang");
  return { enabled: m.enabled === true, collection: scalar(m.collection, "mddb.collection"), sources, ...(lang ? { lang } : {}) };
}
