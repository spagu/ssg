// Where a module lives on this host. A leading underscore keeps this file out
// of the Pages route table.
//
// Every link and every request core makes is built here, from the registry's
// base and a manifest path, and must land on the host the caller asked core
// on. The build already refuses a base with a scheme, a host, `..` or a
// backslash (GO-144); this is the same rule at run time, because the registry
// is a file in the deploy and core does not take its word for it.

const SEGMENT = /^[A-Za-z0-9_~-][A-Za-z0-9._~-]*$/;
// A manifest path: starts with one slash, no backslash, no whitespace. The
// schema is stricter; this is the part that keeps a URL on its host.
const PATH = /^\/(?!\/)[^\\\s]*$/;

// cleanBase returns the base as "" (the site root) or "/a/b", or null when it
// is not a plain path.
export function cleanBase(base: string): string | null {
  if (!base.startsWith("/")) return null;
  const trimmed = base.replace(/\/$/, "");
  if (trimmed === "") return "";
  return trimmed.split("/").slice(1).every((s) => SEGMENT.test(s)) ? trimmed : null;
}

// joinPath is the absolute path of `path` (relative to the module's base) on
// this host, `#fragment` kept, or null when either part is not a plain path.
export function joinPath(base: string, path: string): string | null {
  const prefix = cleanBase(base);
  if (prefix === null || !PATH.test(path)) return null;
  return prefix + path;
}

// targetURL is the URL core may request for a module endpoint: the caller's
// origin, the joined path, nothing else. Anything that would resolve to a
// different origin or path — or carry a query or fragment — is null.
export function targetURL(origin: string, base: string, path: string): URL | null {
  const joined = joinPath(base, path);
  if (joined === null || /[?#]/.test(joined)) return null;
  const url = new URL(joined, origin);
  return url.origin === origin && url.pathname === joined ? url : null;
}

// HREF: the characters a summary's attention link may use
// (module-summary.schema.json is stricter): a path, then an optional
// #fragment for hash routing. No scheme, host, `%`, backslash, whitespace or
// query outside the fragment. Dot segments pass this check on purpose — the
// URL parser below is what refuses them, exactly as a browser would read them.
const HREF = /^\/[A-Za-z0-9._~/-]*(?:#[A-Za-z0-9._~/?=&-]*)?$/;
const PROBE_ORIGIN = "https://core.invalid";

// joinHref is the absolute path on this host of a link a module gave
// relative to its base, or null. The joined path is resolved by the URL
// parser; anything it changes (a `..` or `.` segment that would climb out of
// the base, a second slash that would name a host) is refused, so the link
// shown is the link followed and it stays under the module's base — joinPath
// builds it as base + path, and an unchanged path keeps that prefix.
export function joinHref(base: string, href: string): string | null {
  if (href.length > 256 || !HREF.test(href)) return null;
  const at = href.indexOf("#");
  const path = at < 0 ? href : href.slice(0, at);
  const joined = joinPath(base, path);
  if (joined === null || new URL(joined, PROBE_ORIGIN).href !== PROBE_ORIGIN + joined) return null;
  return joined + (at < 0 ? "" : href.slice(at));
}
