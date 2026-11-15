// Where the MDDB articles of the site are: the server (EDITOR_MDDB_URL), its
// key (the secret EDITOR_MDDB_API_KEY) and the collections — by default the
// ones the site's own configuration builds from (`mddb.collection`,
// `mddb.sources` in the site model: collection, type, section, filter, lang),
// narrowed or named by EDITOR_MDDB_COLLECTION. The url and api_key of
// .ssg.yaml are never read (site-keys.ts): the Worker has its own.
//
// The key goes only to an https:// server that is not on a private or
// loopback address. EDITOR_MDDB_ALLOW_HTTP=true lifts both, for a private
// network — the same opt-in ssg's `mddb.allow_http` is (internal/mddb/client.go).
// A host name that resolves to a private address cannot be seen from here;
// the names that are private by convention are refused.

import type { SiteModel } from "./site-model";
import { validCollection, validLang } from "./mddb-path";

export interface MddbSourceConfig { collection: string; type: "page" | "post" | null; section: string | null; filter: Record<string, string[]> }
export interface MddbConfig {
  url: string; // the base URL, no trailing "/"
  apiKey: string | null; // never in an answer or a log
  sources: MddbSourceConfig[];
  lang: string | null; // the build's language filter (mddb.lang); null = every language
  newLang: string; // the language of a new article
  allowHttp: boolean;
}
export type MddbEnv = {
  EDITOR_MDDB_URL?: string; EDITOR_MDDB_COLLECTION?: string; EDITOR_MDDB_API_KEY?: string;
  EDITOR_MDDB_LANG?: string; EDITOR_MDDB_ALLOW_HTTP?: string | boolean;
};

// mddbConfigured: whether the Worker is meant to read MDDB at all.
// The Worker's Env (http.ts) does not list these; they are read from it as is.
export const mddbConfigured = (raw: object): boolean => {
  const url = (raw as MddbEnv).EDITOR_MDDB_URL;
  return typeof url === "string" && url.trim() !== "";
};

const PRIVATE_NAMES = /(?:^|\.)(?:localhost|local|internal|lan|home\.arpa)$/i;

function privateIPv4(host: string): boolean {
  const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)$/.exec(host);
  if (!m) return false;
  const [a, b] = [Number(m[1]), Number(m[2])];
  return a === 0 || a === 10 || a === 127 || a >= 224 || (a === 169 && b === 254) || (a === 172 && b >= 16 && b <= 31)
    || (a === 192 && b === 168) || (a === 100 && b >= 64 && b <= 127) || (a === 198 && (b === 18 || b === 19));
}

// privateIPv6: loopback, unspecified, unique-local, link-local, and every
// IPv4-mapped or -compatible form (the WHATWG parser rewrites ::ffff:127.0.0.1
// to ::ffff:7f00:1, so the embedded address is not checked — all are refused).
function privateIPv6(host: string): boolean {
  const h = host.toLowerCase();
  return h === "::1" || h === "::" || /^f[cd]/.test(h) || /^fe[89ab]/.test(h) || h.startsWith("::ffff:") || /^::\d/.test(h);
}

// privateHost: a host the key must not be sent to without the switch. The
// host is URL.hostname: IPv4 already in dotted decimal, IPv6 in brackets.
export function privateHost(hostname: string): boolean {
  const h = hostname.replace(/\.$/, "");
  if (h.startsWith("[")) return privateIPv6(h.slice(1, -1));
  return !h.includes(".") || PRIVATE_NAMES.test(h) || privateIPv4(h);
}

// baseUrl: the checked base URL, or the problem (naming the variable only).
function baseUrl(raw: string, allowHttp: boolean): { ok: true; url: string } | { ok: false; problem: string } {
  let u: URL;
  try {
    u = new URL(raw.trim());
  } catch {
    return { ok: false, problem: "EDITOR_MDDB_URL is not a URL" };
  }
  if (u.username || u.password || u.search || u.hash) return { ok: false, problem: "EDITOR_MDDB_URL must not carry credentials, a query or a fragment" };
  if (u.protocol !== "https:" && !(u.protocol === "http:" && allowHttp)) {
    return { ok: false, problem: "EDITOR_MDDB_URL must be https:// (EDITOR_MDDB_ALLOW_HTTP=true allows http:// on a private network)" };
  }
  if (privateHost(u.hostname) && !allowHttp) {
    return { ok: false, problem: "EDITOR_MDDB_URL names a private or loopback host (EDITOR_MDDB_ALLOW_HTTP=true allows it)" };
  }
  return { ok: true, url: (u.origin + u.pathname).replace(/\/+$/, "") };
}

// sourcesOf: the model's sources, narrowed to EDITOR_MDDB_COLLECTION when set.
function sourcesOf(model: SiteModel | null, only: string): MddbSourceConfig[] {
  const m = model?.mddb ?? null;
  const own = (m?.sources.length ? m.sources : m?.collection ? [{ collection: m.collection, type: null, section: null }] : [])
    .filter((s) => s.collection)
    .map((s) => ({ collection: s.collection!, type: s.type as MddbSourceConfig["type"], section: s.section, filter: s.filter ?? {} }));
  if (!only) return own;
  const picked = own.filter((s) => s.collection === only);
  return picked.length ? picked : [{ collection: only, type: null, section: null, filter: {} }];
}

// mddbConfigFrom: the configuration, or the problem naming what to set.
export function mddbConfigFrom(raw: object, model: SiteModel | null): { ok: true; config: MddbConfig } | { ok: false; problem: string } {
  const env = raw as MddbEnv;
  const allowHttp = env.EDITOR_MDDB_ALLOW_HTTP === true || env.EDITOR_MDDB_ALLOW_HTTP === "true";
  const base = baseUrl(env.EDITOR_MDDB_URL ?? "", allowHttp);
  if (!base.ok) return base;
  const sources = sourcesOf(model, (env.EDITOR_MDDB_COLLECTION ?? "").trim());
  if (!sources.length) return { ok: false, problem: "set EDITOR_MDDB_COLLECTION, or mddb.collection / mddb.sources in the site's configuration" };
  const bad = sources.find((s) => !validCollection(s.collection));
  if (bad) return { ok: false, problem: `the MDDB collection "${bad.collection.slice(0, 100)}" is not a plain name` };
  if (sources.some((s) => s.type !== null && s.type !== "page" && s.type !== "post")) return { ok: false, problem: "mddb.sources: type must be page or post" };
  const lang = (env.EDITOR_MDDB_LANG ?? "").trim() || model?.mddb?.lang || null;
  if (lang !== null && !validLang(lang)) return { ok: false, problem: "EDITOR_MDDB_LANG (or mddb.lang) is not a language code" };
  const fallback = model?.defaultLanguage && validLang(model.defaultLanguage) ? model.defaultLanguage : "en";
  const apiKey = typeof env.EDITOR_MDDB_API_KEY === "string" && env.EDITOR_MDDB_API_KEY.trim() ? env.EDITOR_MDDB_API_KEY.trim() : null;
  return { ok: true, config: { url: base.url, apiKey, sources, lang, newLang: lang ?? fallback, allowHttp } };
}
