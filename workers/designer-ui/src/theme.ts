// Where the theme lives, and the two path policies of designer-ui (README of
// the round, decision 7 — the policy is the only place that decides "may this
// file be touched"):
//
//   - THEME: files under <templates_dir>/<template> with an extension on an
//     explicit list. Nothing else — not the overlay, not .ssg.yaml.
//   - SETTINGS: exactly .ssg.designer.json to write; the config files to read.
//
// The theme folder comes from the configuration (file + overlay), as ssg
// joins it (generator.go: filepath.Join(TemplatesDir, Template)). A theme
// that is not in the repository — built into ssg and extracted at build time,
// or downloaded from online_theme — cannot be edited here, and the answer says
// how to bring it into the repository instead of guessing.

import { checkPath, normalizePath } from "../portable/path-policy";
import type { PathPolicy } from "../portable/path-policy";
import { CONFIG_FILES, OVERLAY_FILE } from "./config-read";
import type { ConfigState, Doc } from "./config-read";
import { OpError } from "./ops";

// THEME_EXTENSIONS: text files are read and written as UTF-8, the others as base64.
export const TEXT_EXTENSIONS = [".html", ".css", ".scss", ".js", ".json", ".svg"];
export const BINARY_EXTENSIONS = [".png", ".webp", ".woff2"];
export const THEME_EXTENSIONS = [...TEXT_EXTENSIONS, ...BINARY_EXTENSIONS];

export const SETTINGS_POLICY: PathPolicy = { roots: [], extensions: [], writeFiles: [OVERLAY_FILE], readFiles: CONFIG_FILES };
export const themePolicy = (dir: string): PathPolicy => ({ roots: [dir], extensions: THEME_EXTENSIONS });

// allowedPath: the normalised path a policy lets be written, or 400 saying
// which input (`what`) and why — the only gate on what a publication writes.
export function allowedPath(raw: unknown, policy: PathPolicy, what = "path"): string {
  const v = checkPath(raw, policy, "write");
  if (!v.ok) throw new OpError("path_not_allowed", 400, `${what}: not a file this operation may write (${v.reason})`);
  return v.path;
}

export const isText = (path: string): boolean => TEXT_EXTENSIONS.some((e) => path.toLowerCase().endsWith(e));

export type ThemeLocation =
  | { ok: true; dir: string; template: string }
  | { ok: false; reason: "no_template" | "theme_dir_unknown" | "online_theme"; detail: string; template: string | null };

const text = (v: unknown): string | null => (typeof v === "string" ? v : typeof v === "number" ? String(v) : null);

// themeLocation: the theme folder named by the effective configuration.
export function themeLocation(state: ConfigState, eff: Doc): ThemeLocation {
  for (const key of ["template", "templates_dir", "online_theme"]) {
    if (key in state.unread) return { ok: false, reason: "theme_dir_unknown", template: null, detail: `${state.file}: ${key} is set, but its value could not be read (${state.unread[key]})` };
  }
  if (state.fileProblem && state.format !== null) return { ok: false, reason: "theme_dir_unknown", template: null, detail: state.fileProblem };
  const template = text(eff.template)?.trim() ?? "";
  if (!template) {
    return { ok: false, reason: "no_template", template: null, detail: "template is not set in the configuration (a theme given only on the ssg command line is unknown to the worker); set it in the settings" };
  }
  if (text(eff.online_theme)?.trim()) {
    return { ok: false, reason: "online_theme", template, detail: "the theme is downloaded at build time from online_theme: edit it in its own repository, or copy its files into this repository and remove online_theme" };
  }
  const dirRaw = text(eff.templates_dir) ?? "templates";
  if (dirRaw.startsWith("$")) return { ok: false, reason: "theme_dir_unknown", template, detail: "templates_dir is set from an environment variable at build time" };
  const dir = dirRaw.trim().replace(/^(?:\.\/)+/, "").replace(/\/+$/, "").replace(/^\.$/, ""); // "." = the root
  const v = normalizePath([dir, template].filter(Boolean).join("/"));
  if (!v.ok) return { ok: false, reason: "theme_dir_unknown", template, detail: `templates_dir and template do not name a folder inside the repository (${v.reason})` };
  return { ok: true, dir: v.path, template };
}

// exportHint: how to bring a theme that is not in the repository into it.
export const exportHint = (dir: string, template: string): string =>
  `the theme "${template}" is not in the repository at ${dir}/ — ssg uses its built-in copy and extracts it at build time. ` +
  `To edit it here, run ssg once locally with this configuration (it writes the theme's files to ${dir}/), commit that folder, then reload`;
