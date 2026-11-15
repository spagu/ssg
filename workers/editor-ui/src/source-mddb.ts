// MDDB as a content source: the site's articles kept as documents of the
// collections its build reads (mddb-config.ts), answered in the shapes the
// repository's files are (source.ts). Here: the list, one article, the name
// of a new one; history in mddb-history.ts, publication in mddb-publish.ts.
//
// MDDB requests: the list one search per 500 documents of each source (its
// filter and type sent as filterMeta, sorted by key); an article one get;
// a new name one get per name tried.

import { decodeCursor, encodeCursor, limitOf } from "./content-list";
import type { ListItem } from "./content-list";
import type { Article } from "./content-read";
import { readFields, splitDocument } from "./frontmatter";
import { MAX_CALLS } from "./mddb-client";
import type { MddbDoc } from "./mddb-client";
import type { MddbConfig } from "./mddb-config";
import { docText, summaryOf } from "./mddb-doc";
import { mddbDiff, mddbHistory, mddbReadAt } from "./mddb-history";
import { MDDB_PREFIX, mddbPath, parseMddbPath, validCollection, validLang } from "./mddb-path";
import { publishMddb } from "./mddb-publish";
import { mddbScope, revisionOf, sameLang } from "./mddb-scope";
import type { MddbScope } from "./mddb-scope";
import { OpError } from "../portable/ops";
import type { OpContext } from "../portable/ops";
import { fileSlug } from "./slug";
import type { ContentSource } from "./source";

// SEARCH_PAGE: documents per search (MDDB caps a page at 1000 by default).
export const SEARCH_PAGE = 500;
// MAX_NAMES: names a new article tries before giving up.
export const MAX_NAMES = 10;

// articleOf: a document as the article content_read answers.
export function articleOf(path: string, doc: MddbDoc): Article {
  const { text, hidden } = docText(doc.meta, doc.contentMd);
  const d = splitDocument(text);
  const { fields, fixed } = readFields(d);
  const revision = revisionOf(doc);
  return { path, version: revision, commit: revision, format: d.format, bom: d.bom, frontmatterRaw: d.raw, fields, fixed: [...fixed, ...Object.keys(hidden)], body: d.body, origin: "mddb" };
}

// sectionRef: "mddb:<collection>" or "mddb:<collection>/<lang>" of the site.
export function sectionRef(m: MddbScope, section: string): { collection: string; lang: string | null } {
  const [collection, lang = null, ...rest] = section.slice(MDDB_PREFIX.length).split("/");
  const shaped = validCollection(collection) && (lang === null || validLang(lang)) && !rest.length;
  if (!shaped || !m.sourcesOf(collection).length || (lang !== null && m.config.lang !== null && !sameLang(lang, m.config.lang))) {
    throw new OpError("invalid_path", 400, "not one of the site's MDDB collections or languages", { path: section.slice(0, 400), reason: "outside_roots" });
  }
  return { collection, lang };
}

type Found = { path: string; doc: MddbDoc; section: string; type: ListItem["type"] };

// collect: every document of the asked sources within `budget` searches.
async function collect(m: MddbScope, only: { collection: string; lang: string | null } | null, budget: number): Promise<{ found: Found[]; truncated: boolean }> {
  const byPath = new Map<string, Found>();
  let truncated = false;
  for (const s of m.config.sources.filter((x) => !only || x.collection === only.collection)) {
    const filter = s.type ? s.filter : { ...s.filter, type: ["page", "post"] };
    for (let offset = 0; ; ) {
      if (m.client.used() >= budget) { truncated = true; break; }
      const docs = await m.client.search(s.collection, m.config.lang, filter, offset, SEARCH_PAGE);
      for (const doc of docs) {
        const lang = only?.lang ?? m.config.lang;
        // The build keeps a document of its language, or one without (internal/mddb/client.go filterDocsByLang).
        if ((lang !== null && doc.lang !== "" && !sameLang(doc.lang, lang)) || !m.belongs(s.collection, doc)) continue;
        const p = parseMddbPath(mddbPath({ collection: s.collection, lang: doc.lang, key: doc.key }));
        if (!p.ok || byPath.has(mddbPath(p.ref))) continue; // a key the editor cannot address is not listed
        const path = mddbPath(p.ref);
        byPath.set(path, { path, doc, section: `${MDDB_PREFIX}${s.collection}/${doc.lang}`, type: summaryOf(doc, s.type).type });
      }
      if (docs.length < SEARCH_PAGE) break;
      offset += docs.length;
    }
  }
  return { found: [...byPath.values()].sort((a, b) => (a.path < b.path ? -1 : 1)) /* paths are unique */, truncated };
}

export function mddbSource(ctx: OpContext, config: MddbConfig, hasRepo: boolean): ContentSource {
  const m = mddbScope(config, ctx.deps);
  return {
    origin: "mddb",
    async list(input, budget = MAX_CALLS) {
      const only = input.section ? sectionRef(m, input.section) : null;
      const after = input.cursor ? decodeCursor(input.cursor) : null;
      const { found, truncated } = await collect(m, only, Math.min(budget, MAX_CALLS));
      const q = (input.q ?? "").trim().toLowerCase();
      const matching = found.filter((f) => (!q || f.path.toLowerCase().includes(q)) && (after === null || f.path > after));
      const limit = limitOf(input.limit);
      const page = matching.slice(0, limit);
      const items: ListItem[] = page.map(({ path, doc, section, type }) => {
        const s = summaryOf(doc, type);
        const size = new TextEncoder().encode(m.textOf(doc)).length;
        // MDDB's search gives no revision: content_read gives the version to publish over.
        return { path, version: "", size, section, type, language: doc.lang, title: s.title, date: s.date, draft: s.draft, loaded: true, origin: "mddb" };
      });
      const nextCursor = matching.length > limit ? encodeCursor(page[page.length - 1].path) : null;
      return { items, nextCursor, pending: 0, truncated };
    },
    async read(path) {
      const ref = m.ref(path);
      return articleOf(mddbPath(ref), await m.load(ref));
    },
    async newPath(section, title) {
      const { collection, lang } = sectionRef(m, section);
      const slug = fileSlug(title ?? "");
      for (let n = 1; n <= MAX_NAMES; n++) {
        const ref = { collection, lang: lang ?? config.newLang, key: n === 1 ? slug : `${slug}-${n}` };
        if (!(await m.client.get(ref))) return { path: mddbPath(ref), slug };
      }
      throw new OpError("name_taken", 409, `"${slug}" and the next ${MAX_NAMES - 1} names are taken; choose another title`);
    },
    publish: (body, req) => publishMddb(ctx, m, hasRepo, body, req!),
    history: (path, cursor) => mddbHistory(m, path, cursor),
    readAt: (path, id) => mddbReadAt(m, path, id),
    diff: (input) => mddbDiff(m, input),
  };
}
