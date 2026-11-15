// The theme's files as the Files tab shows them: a folder tree from the flat
// list GET api/theme answers, the edits not yet published (text or a
// replaced image), and the search inside the editor. Pure.

import { lineDiff, stats } from "../core/diff.js";

// IMAGES: the theme's image files the panel can show and replace, by type.
export const IMAGES = { png: "image/png", webp: "image/webp", svg: "image/svg+xml" };
export const extOf = (path) => (/\.([a-z0-9]+)$/i.exec(path)?.[1] ?? "").toLowerCase();
export const isImage = (path) => Object.hasOwn(IMAGES, extOf(path));

const byName = (a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);

// tree: {name, path, dirs: [...], files: [...]} for the files under `dir`,
// folders before files, each sorted by name. A file outside `dir` is listed
// under its full path at the top (never dropped).
export function tree(files, dir) {
  const root = { name: dir, path: dir, dirs: [], files: [] };
  const prefix = dir ? dir + "/" : "";
  for (const f of files) {
    const rel = f.path.startsWith(prefix) ? f.path.slice(prefix.length) : f.path;
    const parts = rel.split("/");
    let at = root;
    for (const name of parts.slice(0, -1)) {
      let next = at.dirs.find((d) => d.name === name);
      if (!next) {
        next = { name, path: `${at.path}/${name}`, dirs: [], files: [] };
        at.dirs.push(next);
      }
      at = next;
    }
    at.files.push({ ...f, name: parts[parts.length - 1] });
  }
  const order = (n) => {
    n.dirs.sort(byName).forEach(order);
    n.files.sort(byName);
    return n;
  };
  return order(root);
}

// createDrafts: the edited files, by path.
//   setText(file, original, text) — a text file's new content; back to the
//   original drops the draft. setImage(file, base64) — a replaced image.
export function createDrafts() {
  const items = new Map();
  return {
    setText(file, original, text) {
      if (text === original) items.delete(file.path);
      else items.set(file.path, { path: file.path, baseBlobId: file.blobId, original, text });
    },
    setImage(file, base64) {
      items.set(file.path, { path: file.path, baseBlobId: file.blobId, base64 });
    },
    get: (path) => items.get(path),
    has: (path) => items.has(path),
    discard: (path) => items.delete(path),
    get size() {
      return items.size;
    },
    items: () => [...items.values()],
    // list: the changes POST api/theme/publish takes.
    list: () => [...items.values()].map((d) => ({ op: "put", path: d.path, content: d.base64 === undefined ? { text: d.text } : { base64: d.base64 }, baseBlobId: d.baseBlobId })),
    clear: () => items.clear(),
  };
}

// describeDraft: {path, image, added, removed} — what the dialog lists.
export function describeDraft(d) {
  if (d.base64 !== undefined) return { path: d.path, image: true, added: 0, removed: 0 };
  return { path: d.path, image: false, ...stats(lineDiff(d.original, d.text)) };
}

// findAll: every match of `query` in `text` ({start, end, line}), ignoring
// case; an empty query finds nothing.
export function findAll(text, query) {
  const out = [];
  if (!query) return out;
  const hay = text.toLowerCase();
  const needle = query.toLowerCase();
  for (let at = hay.indexOf(needle); at >= 0; at = hay.indexOf(needle, at + needle.length)) {
    out.push({ start: at, end: at + needle.length, line: lineAt(text, at) });
  }
  return out;
}

// lineAt: the 1-based line of an offset.
export function lineAt(text, offset) {
  let n = 1;
  for (let i = 0; i < offset && i < text.length; i++) if (text[i] === "\n") n++;
  return n;
}

// lineNumbers: "1\n2\n…" for the gutter beside the editor.
export function lineNumbers(text) {
  const n = String(text).split("\n").length;
  return Array.from({ length: n }, (_, i) => String(i + 1)).join("\n");
}

// nextMatch: the index of the match to go to from the cursor, wrapping;
// -1 when there is none. dir is 1 (next) or -1 (previous).
export function nextMatch(matches, cursor, dir = 1) {
  if (!matches.length) return -1;
  if (dir > 0) {
    const i = matches.findIndex((m) => m.start >= cursor);
    return i >= 0 ? i : 0;
  }
  for (let i = matches.length - 1; i >= 0; i--) if (matches[i].start < cursor) return i;
  return matches.length - 1;
}

// base64Of: the bytes of a file the browser read (ArrayBuffer) as base64.
export function base64Of(buffer) {
  const bytes = new Uint8Array(buffer);
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(bin);
}

// dataUrl: an image's base64 as a data: URL for <img> (img-src allows data:).
export const dataUrl = (path, base64) => `data:${IMAGES[extOf(path)]};base64,${base64}`;
