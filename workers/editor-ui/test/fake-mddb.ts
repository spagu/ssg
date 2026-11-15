// An in-process MDDB for the tests, modelled on tradik/mddb docs/API.md and
// its handlers (services/mddbd: http_handlers.go handleGet/handleAdd,
// document_update.go, revision_handlers.go): POST /v1/get, /v1/add,
// PATCH /v1/update with expectedRevision (409 {"error":"revision conflict",
// expectedRevision, currentRevision}), POST /v1/delete, /v1/search (a bare
// array, no revision; filterMeta AND between keys, OR between values; sort
// by key), POST /v1/revisions (newest first, with content). Identity folds
// ASCII case of key and language. The key is checked as X-API-Key or Bearer.
// No network.

export const MDDB_HOST = "mddb.example.com";
export const MDDB_KEY = "mddb_test_k3y-never-shown";

export type Meta = Record<string, string[]>;
export interface Doc { collection: string; key: string; lang: string; meta: Meta; contentMd: string; addedAt: number; updatedAt: number }
export interface Rev { timestamp: number; updatedAt: number; contentMd: string; meta: Meta }

const idOf = (c: string, k: string, l: string) => `${c}|${k.replace(/[A-Z]/g, (x) => x.toLowerCase())}|${l.replace(/[A-Z]/g, (x) => x.toLowerCase())}`;

// revisionOf: a stand-in for MDDB's hash of content, meta and updatedAt.
export function revisionOf(d: Doc): string {
  let h = 0xcbf29ce484222325n;
  for (const ch of JSON.stringify([d.contentMd, d.meta, d.updatedAt])) h = ((h ^ BigInt(ch.codePointAt(0)!)) * 0x100000001b3n) & 0xffffffffffffffffn;
  return h.toString(16).padStart(16, "0");
}

export class FakeMddb {
  docs = new Map<string, Doc>();
  revs = new Map<string, Rev[]>();
  requests: { method: string; path: string; body: Record<string, unknown>; headers: Headers }[] = [];
  key: string | null = MDDB_KEY;
  clock = 1767225600;
  noRevision = false; // a server before 2.18.8
  // beforeWrite runs before an add/update/delete is applied (to race the editor).
  beforeWrite: ((path: string) => void) | null = null;
  // override answers a request instead of the fake (status codes, bad bodies).
  override: ((path: string, body: Record<string, unknown>) => Response | null) | null = null;

  // put stores a document as a write would (one revision).
  put(collection: string, key: string, lang: string, meta: Meta, contentMd: string): Doc {
    const id = idOf(collection, key, lang);
    const old = this.docs.get(id);
    const now = ++this.clock;
    const doc = { collection, key, lang, meta, contentMd, addedAt: old?.addedAt ?? now, updatedAt: now };
    this.docs.set(id, doc);
    this.revs.set(id, [{ timestamp: now, updatedAt: now, contentMd, meta }, ...(this.revs.get(id) ?? [])]);
    return doc;
  }
  get(collection: string, key: string, lang: string): Doc | undefined {
    return this.docs.get(idOf(collection, key, lang));
  }

  private out(d: Doc, withRevision = true) {
    const { collection, ...rest } = d;
    return { id: idOf(collection, d.key, d.lang), ...rest, ...(withRevision && !this.noRevision ? { revision: revisionOf(d) } : {}) };
  }

  fetch = async (url: string, init: RequestInit = {}): Promise<Response> => {
    const u = new URL(url);
    const method = init.method ?? "GET";
    const headers = new Headers(init.headers);
    const body = JSON.parse(String(init.body ?? "{}")) as Record<string, unknown>;
    this.requests.push({ method, path: u.pathname, body, headers });
    if (u.host !== MDDB_HOST) return new Response("wrong host", { status: 599 });
    if (this.key !== null) {
      const auth = headers.get("authorization") ?? "";
      if (headers.get("x-api-key") !== this.key && auth !== `Bearer ${this.key}`) return Response.json({ error: "unauthorized" }, { status: 401 });
    }
    const forced = this.override?.(u.pathname, body);
    if (forced) return forced;
    const { collection, key, lang } = body as Record<string, string>;
    const id = idOf(String(collection), String(key), String(lang));
    const route = `${method} ${u.pathname}`;
    if (route === "POST /v1/search") return this.search(body);
    if (!collection || !key || !lang) return Response.json({ error: "missing fields" }, { status: 400 });
    const existing = this.docs.get(id);
    const expected = typeof body.expectedRevision === "string" ? body.expectedRevision : "";
    const conflict = () => Response.json({ error: "revision conflict", expectedRevision: expected, currentRevision: existing ? revisionOf(existing) : "" }, { status: 409 });
    switch (route) {
      case "POST /v1/get":
        return existing ? Response.json(this.out(existing)) : Response.json({ error: "document not found" }, { status: 404 });
      case "POST /v1/add":
        this.beforeWrite?.(u.pathname);
        if (expected && (!this.docs.get(id) || revisionOf(this.docs.get(id)!) !== expected)) return conflict();
        return Response.json(this.out(this.put(collection, key, lang, body.meta as Meta, String(body.contentMd ?? ""))));
      case "PATCH /v1/update": {
        this.beforeWrite?.(u.pathname);
        const now = this.docs.get(id);
        if (!now) return Response.json({ error: "document not found" }, { status: 404 });
        if (expected && revisionOf(now) !== expected) return Response.json({ error: "revision conflict", expectedRevision: expected, currentRevision: revisionOf(now) }, { status: 409 });
        return Response.json(this.out(this.put(collection, now.key, now.lang, (body.meta as Meta) ?? now.meta, typeof body.contentMd === "string" ? body.contentMd : now.contentMd)));
      }
      case "POST /v1/delete":
        this.beforeWrite?.(u.pathname);
        if (!this.docs.delete(id)) return Response.json({ error: "document not found" }, { status: 404 });
        return Response.json({ status: "deleted", collection, key, lang });
      case "POST /v1/revisions":
        return Response.json({ collection, key, lang, revisions: this.revs.get(id) ?? [], total: (this.revs.get(id) ?? []).length });
    }
    return Response.json({ error: "not found" }, { status: 404 });
  };

  private search(body: Record<string, unknown>): Response {
    const filter = (body.filterMeta ?? {}) as Meta;
    const all = [...this.docs.values()]
      .filter((d) => d.collection === body.collection && Object.entries(filter).every(([k, want]) => (d.meta[k] ?? []).some((v) => want.includes(v))))
      .sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0));
    const offset = Number(body.offset ?? 0);
    const page = all.slice(offset, offset + Math.min(Number(body.limit ?? 50), 1000));
    return Response.json(page.map((d) => this.out(d, false)), { headers: { "x-total-count": String(all.length) } });
  }
}
