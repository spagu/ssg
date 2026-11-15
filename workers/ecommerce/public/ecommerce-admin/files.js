// Files the panel hands to the person running the shop: the orders CSV and an
// invoice to open or print.
//
// They are fetched with the panel's own credentials (api.file) and saved from
// a Blob, never followed as plain links: in the password mode the password
// lives only in the panel's memory, so a browser navigation would arrive
// without it and the download would fail with a 401. The same code serves the
// Access and `auth` modes, whose cookies the fetch sends anyway.

import { api } from "./api.js";
import { notify } from "./dom.js";

/** The name a file is saved under: the server's path segment, made safe for
 *  every file system (an invoice number like FV/2026/000042 has slashes). */
export function fileName(base, extension) {
  const safe = String(base).replace(/[^A-Za-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "") || "download";
  return `${safe}.${extension}`;
}

/** Saves `blob` as `name` through a temporary link the browser downloads. */
export function saveBlob(blob, name) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.hidden = true;
  document.body.append(link);
  link.click();
  link.remove();
  // Revoked on the next turn, once the browser has taken the download.
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

/** Fetches `path` and saves it as `name`. Never rejects: a failure becomes the
 *  panel's notice, so a click handler can `void` it. */
export async function download(path, name) {
  try {
    saveBlob(await api.file(path), name);
  } catch (err) {
    notify("error", err.message);
  }
}

/** Makes a link that points at a file download through `download` when
 *  clicked, keeping its href for the keyboard, the context menu and the
 *  cookie modes (where opening it in a new tab works on its own). */
export function bindDownload(link, name) {
  link.addEventListener("click", (event) => {
    event.preventDefault();
    void download(link.getAttribute("href"), name);
  });
  return link;
}
