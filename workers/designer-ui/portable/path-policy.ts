// ssg editor-ui — portable: the single place that decides whether a repository path may be read or written
// (README decision 7). Every route, MCP tool and provider call asks checkPath first and then uses the
// normalised path it returns — never the raw input. The worker passes its own roots and extensions; the
// hard denials below hold whatever it passes. Symlinks and submodules are refused by the provider, which
// sees the tree's modes (this module sees only strings).
export type Access = "read" | "write";
// PathPolicy: roots = directories whose files (with an allowed extension) may be read and written;
// extensions lower-case with the dot (".md"); writeFiles / readFiles = exact paths (these may start with a
// dot, e.g. ".ssg.yaml" for reading); maxBytes = UTF-8 length of the whole path (default 400).
export interface PathPolicy { roots: string[]; extensions: string[]; writeFiles?: string[]; readFiles?: string[]; maxBytes?: number }
export type PathRefusal =
  | "not_a_string" | "empty" | "too_long" | "control_character" | "absolute" | "traversal" | "dot_segment"
  | "bad_segment" | "denied" | "outside_roots" | "extension" | "read_only";
export type PathVerdict = { ok: true; path: string } | { ok: false; reason: PathRefusal };

// DENIED_SEGMENTS / DENIED_FILE: never readable or writable, compared case-insensitively (a checkout on a
// case-insensitive disk treats .GitHub as .github). Workflows and Worker code run with secrets.
const DENIED_SEGMENTS = new Set([".github", ".git", "workers", "node_modules"]);
const DENIED_FILE = /^(?:wrangler\.(?:toml|json|jsonc)|\.dev\.vars.*|\.env.*)$/i;
// Control, format (bidi overrides, zero-width), line/paragraph separators, and characters Windows forbids.
const FORBIDDEN = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}:*?"<>|]/u;
const SEGMENT_MAX_BYTES = 255;
const byteLength = (s: string) => new TextEncoder().encode(s).length;

// normalizePath: NFC, "\" → "/", repeated "/" collapsed. Refuses what cannot be normalised safely. The
// result has no leading or trailing "/" and no "." or ".." segment.
export function normalizePath(raw: unknown, maxBytes = 400): PathVerdict {
  if (typeof raw !== "string") return { ok: false, reason: "not_a_string" };
  const s = raw.normalize("NFC").replaceAll("\\", "/");
  if (!s) return { ok: false, reason: "empty" };
  if (FORBIDDEN.test(s)) return { ok: false, reason: "control_character" };
  if (s.startsWith("/")) return { ok: false, reason: "absolute" };
  if (byteLength(s) > maxBytes) return { ok: false, reason: "too_long" };
  const segments = s.split("/").filter((p, i, all) => p !== "" || i === all.length - 1);
  for (const seg of segments) {
    if (seg === "..") return { ok: false, reason: "traversal" };
    if (seg === "" || seg !== seg.trim() || byteLength(seg) > SEGMENT_MAX_BYTES || seg.endsWith(".")) {
      return { ok: false, reason: seg.startsWith(".") ? "dot_segment" : "bad_segment" };
    }
  }
  return { ok: true, path: segments.join("/") };
}

const isDenied = (path: string) => {
  const segs = path.toLowerCase().split("/");
  return segs.some((s) => DENIED_SEGMENTS.has(s)) || DENIED_FILE.test(segs[segs.length - 1]);
};
const normalised = (list: string[] | undefined) =>
  (list ?? []).flatMap((p) => { const v = normalizePath(p); return v.ok ? [v.path] : []; });

// checkPath: may `raw` be accessed for `access` under `policy`? Exact files first (they may be dotfiles),
// then roots + extension; every path also passes the hard denials.
export function checkPath(raw: unknown, policy: PathPolicy, access: Access): PathVerdict {
  const v = normalizePath(raw, policy.maxBytes);
  if (!v.ok) return v;
  const path = v.path;
  if (isDenied(path)) return { ok: false, reason: "denied" };
  if (normalised(policy.writeFiles).includes(path)) return v;
  if (normalised(policy.readFiles).includes(path)) return access === "read" ? v : { ok: false, reason: "read_only" };
  if (path.split("/").some((seg) => seg.startsWith("."))) return { ok: false, reason: "dot_segment" };
  if (!normalised(policy.roots).some((root) => path.startsWith(root + "/"))) return { ok: false, reason: "outside_roots" };
  const name = path.slice(path.lastIndexOf("/") + 1);
  const dot = name.lastIndexOf(".");
  const ext = dot > 0 ? name.slice(dot).toLowerCase() : "";
  if (!ext || !policy.extensions.map((e) => e.toLowerCase()).includes(ext)) return { ok: false, reason: "extension" };
  return v;
}

// checkPrefix: may the directory `raw` be listed? It must be a root or lie inside one ("" is refused).
export function checkPrefix(raw: unknown, policy: PathPolicy): PathVerdict {
  const v = normalizePath(typeof raw === "string" ? raw.replace(/\/+$/, "") : raw, policy.maxBytes);
  if (!v.ok) return v;
  if (isDenied(v.path)) return { ok: false, reason: "denied" };
  if (v.path.split("/").some((seg) => seg.startsWith("."))) return { ok: false, reason: "dot_segment" };
  const inside = normalised(policy.roots).some((root) => v.path === root || v.path.startsWith(root + "/"));
  return inside ? v : { ok: false, reason: "outside_roots" };
}

