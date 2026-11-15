// POST {base}/api/changes for an MDDB article — the same body and the same
// answers as for files (change-publish.ts), within what MDDB can promise:
//
// - One article per publication. MDDB writes one document at a time, so the
//   all-or-nothing of a commit (decision 3) holds only for one.
// - A conflict is the same 409 {error: "conflict", files: [{path, yours,
//   theirs, theirVersion}]}: an edit is a conditional write
//   (PATCH /v1/update with expectedRevision — MDDB answers 409 when the
//   document moved on). Two gaps MDDB leaves (2.19.x): a new article cannot be
//   written "only if absent" (an expectedRevision for a missing document is a
//   conflict, and none is unconditional), and a delete is not conditional;
//   both are checked with a read just before, so only a write landing in
//   between those two requests is not seen.
// - No rename: the key is the article's address in MDDB; its URL on the site
//   is the `slug` field, which the form changes like any other.
// - Images go to the repository (decision 4 of the round): committed first, in
//   their own commit, so the article never names an image that is not there.
//   Without EDITOR_GIT_* they are refused, 503 media_needs_repository.
// - The message is not stored: MDDB keeps no message and no author.
// The answer adds origin "mddb", `rebuild` and `notice` (rebuild.ts);
// commitId is the article's new revision ("" after a delete), branch the
// collection, url the images' commit or "".

import { publish } from "./change-publish";
import type { ConflictFile, Published } from "./change-publish";
import type { ChangeRequest, Content, Planned } from "./change-input";
import { NEW_FILE, applyEdit } from "./frontmatter";
import type { MddbDoc } from "./mddb-client";
import { docText, textDoc } from "./mddb-doc";
import { mddbPath } from "./mddb-path";
import type { MddbRef } from "./mddb-path";
import type { MddbScope } from "./mddb-scope";
import { revisionOf } from "./mddb-scope";
import { OpError } from "./ops";
import type { OpContext } from "./ops";
import { triggerRebuild } from "./rebuild";

const isImage = (c: Planned): boolean => c.op === "put" && "base64" in c.content;
const bad = (detail: string) => new OpError("invalid_change", 400, detail);
const conflict = (files: ConflictFile[]) => new OpError("conflict", 409, "this article changed in MDDB since it was opened", { files });

// yoursOn: the text this change would write over `theirs` (null: a delete,
// or a form edit that does not apply even to a fresh block).
function yoursOn(content: Content | null, theirs: string | null): string | null {
  if (!content) return null;
  if ("text" in content) return content.text;
  const mine = applyEdit(theirs ?? NEW_FILE, content.edit);
  const fresh = mine.refused.length ? applyEdit(NEW_FILE, content.edit) : mine;
  return fresh.refused.length ? null : fresh.text;
}

// conflictWith: the 409 for the document as it is now (null: absent,
// or not one the site reads — then its text is not shown).
function conflictWith(m: MddbScope, path: string, ref: MddbRef, cur: MddbDoc | null, content: Content | null): OpError {
  const visible = cur !== null && m.belongs(ref.collection, cur);
  const theirs = visible ? m.textOf(cur) : null;
  return conflict([{ path, yours: yoursOn(content, theirs), theirs, theirVersion: visible ? revisionOf(cur) : null, ...(cur && !visible ? { theirsUnread: "unavailable" as const } : {}) }]);
}

// planned: the new meta and content of a put, checked before anything is written.
function planned(m: MddbScope, path: string, ref: MddbRef, cur: MddbDoc | null, content: Content) {
  let text: string;
  if ("text" in content) text = content.text;
  else {
    const r = applyEdit(cur ? m.textOf(cur) : NEW_FILE, content.edit);
    if (r.refused.length) throw new OpError("fields_refused", 400, "these frontmatter keys cannot be changed from the form", { path, refused: r.refused });
    text = r.text;
  }
  const doc = textDoc(text, cur ? docText(cur.meta, cur.contentMd).hidden : {});
  if (!doc.ok) throw new OpError("fields_refused", 400, "MDDB keeps flat values only: these keys hold more", { path, refused: doc.refused });
  if (!m.belongs(ref.collection, doc)) throw bad("the article would no longer be one the site builds from (its MDDB source's filter or type)");
  return doc;
}

export async function publishMddb(ctx: OpContext, m: MddbScope, hasRepo: boolean, body: unknown, req: ChangeRequest): Promise<Published> {
  const articles = req.changes.filter((c) => !isImage(c));
  if (articles.length > 1) throw bad("an MDDB publication changes one article; publish the others one by one");
  const change = articles[0];
  if (change.op === "move") throw bad("an MDDB article is not renamed: its key is its address; change its slug to change its URL");
  const images = (body as { changes: unknown[] }).changes.filter((_, i) => isImage(req.changes[i]));
  if (images.length && !hasRepo) {
    throw new OpError("media_needs_repository", 503, "images of MDDB articles are stored in the site's repository: set EDITOR_GIT_REPO and the EDITOR_GIT_TOKEN secret to add them");
  }
  const ref = m.ref(change.path as string);
  const path = mddbPath(ref);
  const content = change.op === "put" ? (change.content as Content) : null;
  const cur = await m.client.get(ref);
  const visible = cur !== null && m.belongs(ref.collection, cur);
  if (change.version === null ? cur !== null : !visible || revisionOf(cur) !== change.version) throw conflictWith(m, path, ref, cur, content);
  const next = content ? planned(m, path, ref, cur, content) : null;

  const pictures = images.length ? await publish(ctx, { message: req.message, changes: images }) : null;
  let revision: string | null = null;
  if (!next) await m.client.remove(ref);
  else if (cur === null) revision = revisionOf(await m.client.add(ref, next.meta, next.contentMd));
  else {
    const w = await m.client.update(ref, next.meta, next.contentMd, change.version!);
    if (!w.ok) throw conflictWith(m, path, ref, "missing" in w ? null : await m.client.get(ref), content);
    revision = revisionOf(w.doc);
  }
  const { rebuild, notice } = await triggerRebuild(ctx);
  return {
    commitId: revision ?? "", url: pictures?.url ?? "", branch: ref.collection,
    files: [{ path, version: revision }, ...(pictures?.files ?? [])], origin: "mddb", rebuild, notice,
  };
}
