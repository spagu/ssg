// An MDDB article's revisions, in the shapes of the repository's
// (revisions.ts, revision-read.ts, revision-diff.ts). MDDB keeps a revision
// per write and lists them with their content (POST /v1/revisions, newest
// first); a revision is named by its `timestamp`, the write's unix second
// (MDDB moves a second write within one second to the next free second).
//
// What MDDB does not keep, the answer leaves empty: a revision has no author
// and no message (authorName, authorEmail, message are ""). Its `version` is
// its timestamp: MDDB gives the revision hash of the current document only.
// Pages of 30 are cut here — MDDB answers the whole history at once, capped
// by the collection's MaxRevisions. Restoring is a publication of the old
// text over the current revision, as for a file (decision 6).
//
// MDDB requests: the document (get) and its history (revisions).

import { diffText } from "./diff";
import type { SideInfo } from "./diff";
import type { MddbRevision } from "./mddb-client";
import { iso } from "./mddb-doc";
import { mddbPath } from "./mddb-path";
import type { MddbScope } from "./mddb-scope";
import { OpError } from "./ops";
import type { DiffInput, DiffSide, RevisionDiff } from "./revision-diff";
import type { RevisionText } from "./revision-read";
import type { Revision, RevisionPage } from "./revisions";

export const HISTORY_PAGE = 30;
const TIMESTAMP = /^[0-9]{1,12}$/;

const dateOf = (r: MddbRevision): string => iso((r.updatedAt || r.timestamp) * 1000);
const entryOf = (r: MddbRevision): Revision => ({ commitId: String(r.timestamp), authorName: "", authorEmail: "", date: dateOf(r), message: "" });

function stamp(raw: unknown, name: string): string {
  if (typeof raw !== "string" || !TIMESTAMP.test(raw)) throw new OpError("invalid_commit", 400, `${name} must be a revision id of this article`, { param: name });
  return raw;
}

// opened: the article's path, document and history.
async function opened(m: MddbScope, raw: string) {
  const ref = m.ref(raw);
  const doc = await m.load(ref);
  return { path: mddbPath(ref), ref, doc, revisions: await m.client.revisions(ref) };
}

function revision(list: MddbRevision[], id: string): MddbRevision {
  const r = list.find((x) => String(x.timestamp) === id);
  if (!r) throw new OpError("revision_not_found", 404, "not a revision of this article", { commit: id });
  return r;
}

export async function mddbHistory(m: MddbScope, raw: string, cursor: string | null): Promise<RevisionPage> {
  if (cursor !== null && !/^[0-9]{1,6}$/.test(cursor)) throw new OpError("bad_cursor", 400, "not a cursor of this file's history");
  const { path, revisions } = await opened(m, raw);
  const from = Number(cursor ?? 0);
  const items = revisions.slice(from, from + HISTORY_PAGE).map(entryOf);
  return { path, items, nextCursor: revisions.length > from + HISTORY_PAGE ? String(from + HISTORY_PAGE) : null };
}

export async function mddbReadAt(m: MddbScope, raw: string, rawId: string): Promise<RevisionText> {
  const id = stamp(rawId, "commit");
  const { path, ref, doc, revisions } = await opened(m, raw);
  const r = revision(revisions, id);
  const text = m.textOf(r);
  const date = dateOf(r);
  return {
    path, commitId: id, authorName: "", authorEmail: "", date, message: "", version: id, text,
    current: { commit: doc.revision, version: doc.revision },
    identical: text === m.textOf(doc),
    restore: { message: `Restore ${ref.key} to the revision of ${date.slice(0, 10)}`.slice(0, 200), change: { op: "put", path, version: doc.revision, text } },
  };
}

export async function mddbDiff(m: MddbScope, input: DiffInput): Promise<RevisionDiff> {
  const from = stamp(input.from, "from");
  const current = input.to === null || input.to === "" || input.to === "current";
  const to = current ? null : stamp(input.to, "to");
  const { path, doc, revisions } = await opened(m, input.path);
  const a = m.textOf(revision(revisions, from));
  const b = to === null ? m.textOf(doc) : m.textOf(revision(revisions, to));
  const d = diffText(a, b);
  const side = (commitId: string, info: SideInfo): DiffSide => ({ commitId, version: commitId, exists: true, ...info });
  const segment = encodeURIComponent(path);
  return {
    path,
    from: side(from, d.from),
    to: { ...side(to ?? doc.revision, d.to), current },
    identical: d.identical,
    tooLarge: d.tooLarge,
    added: d.added,
    removed: d.removed,
    hunks: d.hunks,
    download: { from: `api/revisions/${segment}/${from}`, to: current ? `api/content/${segment}` : `api/revisions/${segment}/${to}` },
  };
}
