// Drafts and waiting images in this browser (IndexedDB, database
// "ssg-editor"): a draft is the article document (models/article-doc.js) with
// the time it was saved; an image is its bytes, type, alt text and canonical
// path, stored under the draft it was pasted into. Nothing here goes to the
// worker: a draft lives on this device only (README of the round: no shared
// drafts in v1).
//
// IndexedDB can be missing or refuse to open (a private window, blocked site
// data). The store then keeps everything in memory and says so
// (`persistent: false`), and the panel warns that a reload loses the draft.
//
// rescue: one synchronous copy of the open draft's text in localStorage,
// written when the page is being left — an IndexedDB write started then may
// not finish before the page is gone; a localStorage write always does.

const NAME = "ssg-editor";
const imageId = (draft, path) => `${draft}\n${path}`;

const done = (r) => new Promise((resolve, reject) => {
  r.onsuccess = () => resolve(r.result);
  r.onerror = () => reject(r.error);
});

function memory() {
  const drafts = new Map();
  const images = new Map();
  return {
    persistent: false,
    getDraft: async (key) => drafts.get(key) ?? null,
    putDraft: async (rec) => void drafts.set(rec.key, rec),
    listDrafts: async () => [...drafts.values()],
    deleteOnly: async (key) => void drafts.delete(key),
    putImage: async (rec) => void images.set(imageId(rec.draft, rec.path), rec),
    allImages: async () => [...images.values()],
    deleteImage: async (draft, path) => void images.delete(imageId(draft, path)),
  };
}

function indexed(db) {
  const run = (store, mode, fn) => new Promise((resolve, reject) => {
    const tx = db.transaction(store, mode);
    const r = fn(tx.objectStore(store));
    tx.oncomplete = () => resolve(r.result);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
  return {
    persistent: true,
    getDraft: async (key) => (await run("drafts", "readonly", (s) => s.get(key))) ?? null,
    putDraft: async (rec) => void (await run("drafts", "readwrite", (s) => s.put(rec))),
    listDrafts: () => run("drafts", "readonly", (s) => s.getAll()),
    deleteOnly: async (key) => void (await run("drafts", "readwrite", (s) => s.delete(key))),
    putImage: async (rec) => void (await run("images", "readwrite", (s) => s.put({ ...rec, id: imageId(rec.draft, rec.path) }))),
    allImages: () => run("images", "readonly", (s) => s.getAll()),
    deleteImage: async (draft, path) => void (await run("images", "readwrite", (s) => s.delete(imageId(draft, path)))),
  };
}

// openStore: the store on `idb` (window.indexedDB), or in memory.
export async function openStore(idb = globalThis.indexedDB) {
  let base;
  try {
    const r = idb.open(NAME, 1);
    r.onupgradeneeded = () => {
      r.result.createObjectStore("drafts", { keyPath: "key" });
      r.result.createObjectStore("images", { keyPath: "id" });
    };
    base = indexed(await done(r));
  } catch {
    base = memory();
  }
  return {
    ...base,
    // imagesOf: the waiting images of one draft.
    imagesOf: async (draft) => (await base.allImages()).filter((i) => i.draft === draft),
    // deleteDraft: the draft and every image waiting with it.
    async deleteDraft(key) {
      for (const img of (await base.allImages()).filter((i) => i.draft === key)) await base.deleteImage(key, img.path);
      await base.deleteOnly(key);
    },
  };
}

const RESCUE = "ssg-editor-rescue";

// writeRescue: the draft's record, synchronously; false when storage refuses.
export function writeRescue(storage, rec) {
  try {
    storage.setItem(RESCUE, JSON.stringify(rec));
    return true;
  } catch {
    return false;
  }
}

// takeRescue: the rescued record for `key` (removed once read), else null.
export function takeRescue(storage, key) {
  try {
    const rec = JSON.parse(storage.getItem(RESCUE) ?? "null");
    if (!rec || rec.key !== key) return null;
    storage.removeItem(RESCUE);
    return rec;
  } catch {
    return null;
  }
}

// newer: whichever of two records of one draft was saved last.
export const newer = (a, b) => (!a ? b : !b ? a : String(b.savedAt) > String(a.savedAt) ? b : a);
