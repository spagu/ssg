// What every MDDB operation starts from: the client, the configuration, and
// the checks that keep the editor inside the site — a path names one of the
// configured collections (and the build's language, when it filters by one),
// and the document is one a source of the site reads (its filter, its type).
// A document outside is "not found", as a file outside the content folders is
// not listed: the editor never shows what the site does not build from.

import { mddbClient } from "./mddb-client";
import type { MddbClient, MddbDoc } from "./mddb-client";
import type { MddbConfig, MddbSourceConfig } from "./mddb-config";
import { docText, matchesFilter } from "./mddb-doc";
import { parseMddbPath } from "./mddb-path";
import type { MddbRef } from "./mddb-path";
import { OpError } from "../portable/ops";
import type { Deps } from "../portable/ops";

export interface MddbScope {
  client: MddbClient;
  config: MddbConfig;
  ref(raw: string): MddbRef; // 400 invalid_path when not an article of the site
  sourcesOf(collection: string): MddbSourceConfig[];
  belongs(collection: string, doc: { meta: MddbDoc["meta"] }): boolean;
  load(ref: MddbRef): Promise<MddbDoc>; // 404 not_found when absent or outside
  textOf(doc: Pick<MddbDoc, "meta" | "contentMd">): string;
}

export const sameLang = (a: string, b: string): boolean => a.toLowerCase() === b.toLowerCase();

// revisionOf: the version of a document; a server older than MDDB 2.18.8
// gives none, and without it no write can be conditional.
export function revisionOf(doc: MddbDoc): string {
  if (!doc.revision) throw new OpError("mddb_too_old", 503, "this MDDB server gives no document revision; the editor needs MDDB 2.18.8 or later");
  return doc.revision;
}

export function mddbScope(config: MddbConfig, deps: Deps): MddbScope {
  const client = mddbClient(config, deps);
  const sourcesOf = (collection: string) => config.sources.filter((s) => s.collection === collection);
  const belongs = (collection: string, doc: { meta: MddbDoc["meta"] }) => sourcesOf(collection).some((s) => matchesFilter(doc.meta, s.filter, s.type));
  return {
    client,
    config,
    sourcesOf,
    belongs,
    ref(raw) {
      const p = parseMddbPath(raw);
      if (!p.ok) throw new OpError("invalid_path", 400, p.reason, { path: raw.slice(0, 400), reason: "mddb_path" });
      if (!sourcesOf(p.ref.collection).length || (config.lang !== null && !sameLang(p.ref.lang, config.lang))) {
        throw new OpError("invalid_path", 400, "not one of the site's MDDB collections or languages", { path: raw.slice(0, 400), reason: "outside_roots" });
      }
      return p.ref;
    },
    async load(ref) {
      const doc = await client.get(ref);
      if (!doc || !belongs(ref.collection, doc)) throw new OpError("not_found", 404, "no such article in MDDB");
      revisionOf(doc);
      return doc;
    },
    textOf: (doc) => docText(doc.meta, doc.contentMd).text,
  };
}
