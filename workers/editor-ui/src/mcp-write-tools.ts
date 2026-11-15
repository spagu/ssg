// The MCP tools that write: each call is ONE publication through
// change_publish (change-publish.ts) — one commit with every file in it and
// the caller's `message` — so an assistant never makes a commit per file.
// A call takes `changes` (several files) or one file's fields at the top
// level, as `ssg mcp` takes them. The body sent to change_publish is the one
// POST /api/changes takes, so both refuse alike (permissions, paths,
// versions, conflicts, sizes).

import { changePublishOp } from "./change-publish";
import { contentNewPathOp } from "./content-new";
import type { NewPath } from "./content-new";
import { decodeBase64 } from "./media-change";
import { mediaNewPathOp } from "./media-list";
import { sha256Hex, sniffImage } from "./media-name";
import { invalid, items, s, schema, str } from "./mcp-args";
import type { EditorTool } from "./mcp-args";
import { PERM } from "./permissions";
import type { Call, Json } from "../portable/mcp-transport";
import type { Op } from "./ops";

const MESSAGE = s("The commit message: what changed and why. First line at most 200 characters");
const PATH = s("Repository path of the Markdown file, e.g. content/blog/posts/hello.md");
const VERSION = s("The file's version from content_read (or media_list); refused as conflict when it moved on");
const ONE_COMMIT = " One call = one commit: give several files in `changes` rather than one call per file.";
const WRITE = { readOnlyHint: false, destructiveHint: false, idempotentHint: false, openWorldHint: false };
const DESTROY = { ...WRITE, destructiveHint: true };

// many: the schema of a write — the files as `changes`, or one at the top level.
function many(file: Json, required: string[]): Json {
  const item = schema(file, ...required);
  return schema({ message: MESSAGE, ...file, changes: { type: "array", minItems: 1, items: item, description: "Several files in the same commit" } }, "message");
}

const publication = (args: Json, changes: Json[]) => ({ message: args.message, changes });

// newContentPath: the path given, else the one content_new_path names from section + title.
async function newContentPath(c: Json, call: Call<Op>): Promise<unknown> {
  if (c.path !== undefined) return c.path;
  return ((await call(contentNewPathOp, { section: str(c, "section"), title: str(c, "title") })) as NewPath).path;
}

// newImagePath: the path given, else the canonical one for these bytes
// (media_new_path: <mediaDir>/<yyyy>/<mm>/<slug>-<hash8>.<ext>). Bytes that
// are no image are still named, as PNG: change_publish refuses them.
async function newImagePath(c: Json, base64: string, call: Call<Op>): Promise<unknown> {
  if (c.path !== undefined) return c.path;
  const bytes = decodeBase64(base64);
  const input = { name: str(c, "name") ?? "image", type: sniffImage(bytes) ?? "png", hash: await sha256Hex(bytes) };
  return ((await call(mediaNewPathOp, input)) as { path: string }).path;
}

async function each(list: Json[], f: (c: Json) => Promise<Json> | Json): Promise<Json[]> {
  const out: Json[] = [];
  for (const c of list) out.push(await f(c)); // one after another: the subrequest budget is per request
  return out;
}

export const WRITE_TOOLS: EditorTool[] = [
  {
    name: "content_create",
    description: "CONTENT: Create NEW Markdown files: the full document (YAML front matter and body) in `content`. Give " +
      "`path`, or `section` (a content folder) and `title` for a path made with ssg's slug. Refused when the path " +
      "exists (use content_update)." + ONE_COMMIT,
    inputSchema: many({ path: PATH, section: s("A content folder, e.g. content/blog/posts (with title, instead of path)"),
      title: s("The title the file name is made from (with section)"), content: s("The complete Markdown document") }, ["content"]),
    annotations: WRITE,
    op: changePublishOp,
    input: async (a, call) => publication(a, await each(items(a, ["path", "section", "title", "content"]), async (c) =>
      ({ op: "put", path: await newContentPath(c, call), version: null, text: c.content }))),
  },
  {
    name: "content_update",
    description: "CONTENT: Replace EXISTING Markdown files with their full new text (not a patch), each with the " +
      "`version` content_read gave. With `to`, the file is also moved (renamed); `content` is then optional." + ONE_COMMIT,
    inputSchema: many({ path: PATH, version: VERSION, content: s("The complete new document"), to: s("A new path, to move the file") }, ["path", "version"]),
    annotations: WRITE,
    op: changePublishOp,
    input: (a) => publication(a, items(a, ["path", "version", "content", "to"]).map((c) => (c.to === undefined
      ? { op: "put", path: c.path, version: c.version, text: c.content }
      : { op: "move", from: c.path, to: c.to, version: c.version, ...(c.content === undefined ? {} : { text: c.content }) }))),
  },
  {
    name: "content_delete",
    description: "CONTENT: Remove Markdown files, each with its `version`. Destructive: only when explicitly asked." + ONE_COMMIT,
    inputSchema: many({ path: PATH, version: VERSION }, ["path", "version"]),
    annotations: DESTROY,
    permissions: [PERM.publish, PERM.delete],
    op: changePublishOp,
    input: (a) => publication(a, items(a, ["path", "version"]).map((c) => ({ op: "delete", path: c.path, version: c.version }))),
  },
  {
    name: "media_upload",
    description: "MEDIA: Add NEW images from base64 bytes: PNG, JPEG, WebP, GIF or AVIF, decided by the bytes (SVG and " +
      "markup are refused). The canonical path is chosen from `name` and the bytes' hash; the answer lists it; link it " +
      "from an article with content_update. The same bytes already on the branch are kept, not added twice." + ONE_COMMIT,
    inputSchema: many({ content_base64: s("The image's bytes, base64-encoded"), name: s("The original file name or alt text the slug is made from"),
      path: s("Only to repeat a canonical path media_upload or the panel computed") }, ["content_base64"]),
    annotations: WRITE,
    permissions: [PERM.publish, PERM.media],
    op: changePublishOp,
    input: async (a, call) => publication(a, await each(items(a, ["content_base64", "name", "path"]), async (c) => {
      if (typeof c.content_base64 !== "string") throw invalid("content_base64 must be the image's bytes, base64-encoded");
      return { op: "put", path: await newImagePath(c, c.content_base64, call), version: null, base64: c.content_base64 };
    })),
  },
  {
    name: "media_delete",
    description: "MEDIA: Remove images by repository path (from media_list), each with its `version`. Pages that still " +
      "link the image are not checked: look first. Destructive: only when explicitly asked." + ONE_COMMIT,
    inputSchema: many({ path: s("Repository path of the image, from media_list"), version: VERSION }, ["path", "version"]),
    annotations: DESTROY,
    permissions: [PERM.publish, PERM.delete],
    op: changePublishOp,
    input: (a) => publication(a, items(a, ["path", "version"]).map((c) => ({ op: "delete", path: c.path, version: c.version }))),
  },
];
