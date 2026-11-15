// GET {base}/api/content/{path} — one article, split for the editor:
//   {path, version, commit, format, bom, frontmatterRaw, fields, fixed, body}
// `version` is the file's blob id — send it back with a change, so a file
// that moved on in between is a conflict, not an overwrite (decision 4).
// `fields` are the simple top-level keys the form edits, `fixed` the keys it
// may not change; `frontmatterRaw` and `body` joined with the fences give the
// file back byte for byte. `{path}` is the repository path, URL-encoded as one
// segment (content%2Fblog%2Fposts%2Fhello.md).
//
// Requests to the repository host: the head, the file's folder listing and
// its blob.

import { contentPath } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { decodeText, metaOf, remember } from "./content-meta";
import { readFields, splitDocument } from "./frontmatter";
import type { FieldValue, Format } from "./frontmatter";
import { OpError } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { sourceFor } from "./source";

export interface Article {
  path: string;
  version: string;
  commit: string;
  format: Format;
  bom: boolean; // true: ssg sees the BOM before the fence and reads no frontmatter
  frontmatterRaw: string;
  fields: Record<string, FieldValue>;
  fixed: string[];
  body: string;
  origin?: "mddb"; // only on an MDDB article (source-mddb.ts)
}

// readArticle: a repository file as an article (source-git.ts).
export async function readArticle(s: ContentScope, raw: string): Promise<Article> {
  const path = contentPath(raw, s, "read");
  const file = await s.repo.provider.read(s.model.head, path);
  const text = decodeText(file.bytes);
  if (text === null) throw new OpError("not_text", 422, "the file is not UTF-8 text");
  remember(file.blobId, metaOf(text));
  const d = splitDocument(text);
  const { fields, fixed } = readFields(d);
  return { path, version: file.blobId, commit: s.model.head, format: d.format, bom: d.bom, frontmatterRaw: d.raw, fields, fixed, body: d.body };
}

export const contentReadOp: Op<{ path: string }, Article> = {
  name: "content_read",
  permission: PERM.read,
  run: async (ctx, input) => (await sourceFor(ctx, input.path)).read(input.path),
};
