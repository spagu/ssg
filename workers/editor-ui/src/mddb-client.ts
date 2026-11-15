// The few MDDB calls the editor makes (tradik/mddb docs/API.md):
//   POST  /v1/get            one document, with its `revision` (v2.18.8+)
//   POST  /v1/add            create (or overwrite); expectedRevision optional
//   PATCH /v1/update         conditional write: expectedRevision → 409
//                            {"error":"revision conflict", currentRevision}
//   POST  /v1/delete         remove (not conditional: the caller checks first)
//   POST  /v1/search         a page of a collection, filterMeta, sort by key
//   POST  /v1/revisions      a document's history, newest first, with content
//
// The key is sent as ssg sends it (internal/mddb/client.go setCredential): a
// JWT as `Authorization: Bearer`, anything else as `X-API-Key`. It is never
// put in an error, an answer or a log, and redirects are never followed — a
// 3xx could carry the key's next request to another host.

import type { MddbConfig } from "./mddb-config";
import type { MddbRef } from "./mddb-path";
import { OpError } from "../portable/ops";
import type { Deps } from "../portable/ops";

export type Meta = Record<string, string[]>;
export interface MddbDoc { key: string; lang: string; meta: Meta; contentMd: string; addedAt: number; updatedAt: number; revision: string }
export interface MddbRevision { timestamp: number; updatedAt: number; contentMd: string; meta: Meta }
export type Written = { ok: true; doc: MddbDoc } | { ok: false; conflict: string | null } | { ok: false; missing: true };

// MAX_CALLS: MDDB requests one editor request may make, beside the
// repository's own budget (repo.ts); MAX_RESPONSE: bytes of one answer.
export const MAX_CALLS = 20;
export const MAX_RESPONSE = 32 * 1024 * 1024;
export const TIMEOUT_MS = 15_000;

const JWT = /^[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+$/;

// setOwn stores a key as data: a meta key may be "__proto__", which an
// assignment would take as the object's prototype.
export const setOwn = <T>(o: Record<string, T>, k: string, value: T): void => {
  Object.defineProperty(o, k, { value, enumerable: true, writable: true, configurable: true });
};

// metaOf: a meta object as MDDB stores it (string lists); other values are dropped.
function metaOf(v: unknown): Meta {
  const out: Meta = {};
  if (typeof v !== "object" || v === null) return out;
  for (const [k, list] of Object.entries(v)) {
    if (Array.isArray(list)) setOwn(out, k, list.filter((x) => typeof x === "string" || typeof x === "number" || typeof x === "boolean").map(String));
  }
  return out;
}
const num = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) ? v : 0);
const str = (v: unknown): string => (typeof v === "string" ? v : "");
const docOf = (d: Record<string, unknown>): MddbDoc => ({
  key: str(d.key), lang: str(d.lang), meta: metaOf(d.meta), contentMd: str(d.contentMd),
  addedAt: num(d.addedAt), updatedAt: num(d.updatedAt), revision: str(d.revision),
});

const fail = (code: string, status: number, detail: string) => new OpError(code, status, detail);

export interface MddbClient {
  get(ref: MddbRef): Promise<MddbDoc | null>;
  add(ref: MddbRef, meta: Meta, contentMd: string): Promise<MddbDoc>;
  update(ref: MddbRef, meta: Meta, contentMd: string, expected: string): Promise<Written>;
  remove(ref: MddbRef): Promise<void>;
  search(collection: string, lang: string | null, filter: Meta, offset: number, limit: number): Promise<MddbDoc[]>;
  revisions(ref: MddbRef): Promise<MddbRevision[]>;
  used(): number;
}

export function mddbClient(config: MddbConfig, deps: Deps): MddbClient {
  const doFetch = deps.fetch ?? ((url: string, init?: RequestInit) => fetch(url, init));
  let calls = 0;

  // call: status and parsed body; transport failures are thrown as OpError.
  async function call(method: string, path: string, body: unknown): Promise<{ status: number; data: unknown }> {
    if (calls >= MAX_CALLS) throw fail("mddb_budget", 503, "this request needs more MDDB calls than one request may make");
    calls++;
    const headers: Record<string, string> = { "content-type": "application/json", accept: "application/json" };
    if (config.apiKey) headers[JWT.test(config.apiKey) ? "authorization" : "x-api-key"] = JWT.test(config.apiKey) ? `Bearer ${config.apiKey}` : config.apiKey;
    let res: Response;
    try {
      res = await doFetch(config.url + path, { method, headers, body: JSON.stringify(body), redirect: "manual", signal: AbortSignal.timeout(TIMEOUT_MS) });
    } catch {
      throw fail("mddb_unavailable", 502, "the MDDB server did not answer");
    }
    if (res.status >= 300 && res.status < 400) throw fail("mddb_error", 502, "the MDDB server answered with a redirect; redirects are not followed");
    if (res.status === 401 || res.status === 403) throw fail("mddb_rejected", 502, "the MDDB server refused the request: check EDITOR_MDDB_API_KEY and its permissions on the collection");
    if (res.status === 429) throw fail("rate_limited", 429, "the MDDB server is limiting requests; try again later");
    if (res.status >= 500) throw fail("mddb_unavailable", 502, `the MDDB server failed (${res.status})`);
    if (Number(res.headers.get("content-length") ?? 0) > MAX_RESPONSE) throw fail("mddb_error", 502, "the MDDB answer is too large");
    const text = await res.text();
    if (text.length > MAX_RESPONSE) throw fail("mddb_error", 502, "the MDDB answer is too large");
    try {
      return { status: res.status, data: JSON.parse(text) };
    } catch {
      if (res.status >= 400) return { status: res.status, data: null };
      throw fail("mddb_error", 502, "the MDDB answer is not JSON");
    }
  }
  const object = (data: unknown): Record<string, unknown> => {
    if (typeof data !== "object" || data === null || Array.isArray(data)) throw fail("mddb_error", 502, "the MDDB answer has an unexpected shape");
    return data as Record<string, unknown>;
  };
  // refused: a 4xx MDDB gave for a request the editor built (a schema
  // refusal, a read-only collection): its message is MDDB's, not the key.
  const refused = (r: { status: number; data: unknown }): OpError => {
    const said = typeof r.data === "object" && r.data !== null ? str((r.data as Record<string, unknown>).error).slice(0, 300) : "";
    return new OpError("mddb_refused", 422, said ? `MDDB refused the request: ${said}` : `MDDB refused the request (${r.status})`);
  };
  const id = (ref: MddbRef) => ({ collection: ref.collection, key: ref.key, lang: ref.lang });

  return {
    async get(ref) {
      const r = await call("POST", "/v1/get", id(ref));
      if (r.status === 404) return null;
      if (r.status !== 200) throw refused(r);
      return docOf(object(r.data));
    },
    async add(ref, meta, contentMd) {
      const r = await call("POST", "/v1/add", { ...id(ref), meta, contentMd });
      if (r.status !== 200) throw refused(r);
      return docOf(object(r.data));
    },
    async update(ref, meta, contentMd, expected) {
      const r = await call("PATCH", "/v1/update", { ...id(ref), meta, contentMd, expectedRevision: expected });
      if (r.status === 404) return { ok: false, missing: true };
      if (r.status === 409) return { ok: false, conflict: str((r.data as Record<string, unknown> | null)?.currentRevision) || null };
      if (r.status !== 200) throw refused(r);
      return { ok: true, doc: docOf(object(r.data)) };
    },
    async remove(ref) {
      const r = await call("POST", "/v1/delete", id(ref));
      if (r.status !== 200 && r.status !== 404) throw refused(r);
    },
    async search(collection, lang, filter, offset, limit) {
      const r = await call("POST", "/v1/search", { collection, ...(lang ? { lang } : {}), filterMeta: filter, sort: "key", asc: true, limit, offset });
      if (r.status === 404) return [];
      if (r.status !== 200 || !Array.isArray(r.data)) throw fail("mddb_error", 502, "the MDDB search answer has an unexpected shape");
      return r.data.map((d) => docOf(object(d)));
    },
    async revisions(ref) {
      const r = await call("POST", "/v1/revisions", id(ref));
      if (r.status === 404) return [];
      if (r.status !== 200) throw fail("mddb_error", 502, "the MDDB revisions answer has an unexpected shape");
      const list = object(r.data).revisions;
      return (Array.isArray(list) ? list : []).map((e) => {
        const o = object(e);
        return { timestamp: num(o.timestamp), updatedAt: num(o.updatedAt), contentMd: str(o.contentMd), meta: metaOf(o.meta) };
      });
    },
    used: () => calls,
  };
}
