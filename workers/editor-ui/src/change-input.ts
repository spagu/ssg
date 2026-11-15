// The body of POST {base}/api/changes, read and checked before any request to
// the repository host:
//
//   {message: "Fix the intro",
//    changes: [
//      {op: "put",    path, version: "<blob id>" | null, text}            // whole file
//      {op: "put",    path, version: "<blob id>", fields?, frontmatterRaw?, body?}  // form edit
//      {op: "put",    path, version: null, fields?, body?}               // new file from the form
//      {op: "put",    path, version: null, base64}                       // an image (media-change.ts)
//      {op: "move",   from, to, version, text? | fields? frontmatterRaw? body?}
//      {op: "delete", path, version}]}                                   // a content file or an image
//
// `version` is what contentRead answered; null means "this file must not
// exist yet". The permission of each kind is checked here, so a refusal costs
// nothing: put and move need editor-ui:publish (the operation's own), delete
// needs editor-ui:delete as well, an image put editor-ui:media as well — one
// change the caller may not make refuses the whole publication.

import { readLimited } from "../portable/mcp-transport";
import { sanitizeMessage } from "../portable/repo-provider";
import type { Edit } from "./frontmatter";
import { OpError } from "./ops";
import type { OpContext } from "./ops";
import { PERM } from "./permissions";
import { can } from "./session-verifier";

export type Content = { text: string } | { edit: Edit };
export type Image = { base64: string };
export type Planned =
  | { op: "put"; path: unknown; version: string | null; content: Content | Image }
  | { op: "move"; from: unknown; to: unknown; version: string; content: Content | null }
  | { op: "delete"; path: unknown; version: string };
export interface ChangeRequest { message: string; changes: Planned[] }

// MAX_BODY: the request body's limit — a change at the Free plan's 2 MB limit
// (portable/repo-provider.ts, the limits repo.ts uses), escaped into JSON,
// stays under it; the provider enforces the real limits per file and change.
export const MAX_BODY = 8 * 1024 * 1024;
export const MAX_FIRST_LINE = 200;

const bad = (detail: string, index?: number) => new OpError("invalid_change", 400, detail, index === undefined ? {} : { index });
const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const wellFormed = (s: string): boolean => !/\p{Cs}/u.test(s);
const text = (v: unknown, what: string, i: number): string => {
  if (typeof v !== "string" || !wellFormed(v)) throw bad(`${what} must be UTF-8 text`, i);
  return v;
};

// readBody: the request's JSON object (415 for another type, 413 when too large).
export async function readBody(ctx: OpContext): Promise<unknown> {
  if (!/^application\/json\b/i.test(ctx.request.headers.get("content-type") ?? "")) {
    throw new OpError("unsupported_media_type", 415, "send application/json");
  }
  // Streamed with a running byte count (SR-14): a chunked body is not held
  // whole before its size is known.
  const raw = await readLimited(ctx.request, MAX_BODY);
  if (raw === null) throw new OpError("too_large", 413, "the request body is too large");
  try {
    return JSON.parse(raw);
  } catch {
    throw new OpError("bad_json", 400, "the body is not JSON");
  }
}

// contentOf: the new content of a put or move — whole text, or a form edit.
function contentOf(c: Record<string, unknown>, i: number, optional: boolean): Content | null {
  const edit: Edit = {};
  if (c.fields !== undefined) {
    if (!isObject(c.fields)) throw bad("fields must be an object", i);
    edit.fields = c.fields;
  }
  if (c.frontmatterRaw !== undefined) edit.frontmatterRaw = text(c.frontmatterRaw, "frontmatterRaw", i);
  if (c.body !== undefined) edit.body = text(c.body, "body", i);
  const isEdit = Object.keys(edit).length > 0;
  if (c.text !== undefined && isEdit) throw bad("give text or fields/frontmatterRaw/body, not both", i);
  if (c.text !== undefined) return { text: text(c.text, "text", i) };
  if (isEdit) return { edit };
  if (optional) return null;
  throw bad("give the new content: text, or fields/frontmatterRaw/body", i);
}

function version(v: unknown, i: number, nullable: boolean): string | null {
  if (v === null && nullable) return null;
  if (typeof v === "string" && v) return v;
  throw bad(nullable ? "version must be the file's version, or null for a new file" : "version must be the file's version", i);
}

// imageOf: an image put — editor-ui:media, base64 only, always a new file
// (a changed image gets a new name: media-name.ts).
function imageOf(c: Record<string, unknown>, i: number, perms: string[]): Image {
  if (!can(perms, PERM.media)) throw new OpError("forbidden", 403, undefined, { permission: PERM.media });
  if (typeof c.base64 !== "string" || !c.base64) throw bad("base64 must be the image's bytes, base64-encoded", i);
  if (["text", "fields", "frontmatterRaw", "body"].some((k) => c[k] !== undefined)) throw bad("an image carries base64 only", i);
  if (c.version !== null) throw bad("an image is always new: version must be null", i);
  return { base64: c.base64 };
}

// parseChange reads one change; `perms` decides whether a delete is allowed.
function parseChange(c: unknown, i: number, perms: string[]): Planned {
  if (!isObject(c)) throw bad("a change must be an object", i);
  if (c.op === "put") {
    const v = version(c.version, i, true);
    return { op: "put", path: c.path, version: v, content: c.base64 === undefined ? contentOf(c, i, false)! : imageOf(c, i, perms) };
  }
  if (c.op === "move" && c.base64 !== undefined) throw bad("an image is not moved; add it under its new name", i);
  if (c.op === "move") return { op: "move", from: c.from, to: c.to, version: version(c.version, i, false)!, content: contentOf(c, i, true) };
  if (c.op === "delete") {
    if (!can(perms, PERM.delete)) throw new OpError("forbidden", 403, undefined, { permission: PERM.delete });
    return { op: "delete", path: c.path, version: version(c.version, i, false)! };
  }
  throw bad('op must be "put", "move" or "delete"', i);
}

// parseRequest checks the message and every change.
export function parseRequest(body: unknown, perms: string[]): ChangeRequest {
  if (!isObject(body)) throw bad("the body must be an object");
  if (typeof body.message !== "string" || !wellFormed(body.message)) throw new OpError("invalid_message", 400, "message must be UTF-8 text");
  const message = sanitizeMessage(body.message);
  if ([...message.split("\n")[0]].length > MAX_FIRST_LINE) {
    throw new OpError("invalid_message", 400, `the first line of the message may have at most ${MAX_FIRST_LINE} characters`);
  }
  if (!Array.isArray(body.changes) || body.changes.length === 0) throw bad("changes must be a non-empty list");
  return { message, changes: body.changes.map((c, i) => parseChange(c, i, perms)) };
}
