// GO-173: drafts and waiting images in the browser (public/js/store/
// drafts-db.js) — on IndexedDB (an in-process stand-in below, the parts of
// the API the store uses, answering asynchronously as the browser does), in
// memory when IndexedDB is missing or refuses, and the synchronous rescue
// copy written when the page is left.
import { test } from "node:test";
import assert from "node:assert/strict";
import { newer, openStore, takeRescue, writeRescue } from "../public/js/store/drafts-db.js";

type Rec = Record<string, unknown>;
// fakeIndexedDB: databases of object stores (Map by keyPath), requests and
// transactions that complete on a later turn. `fail` makes open refuse;
// state.abort = "abort" | "error" makes the next transaction end so.
export function fakeIndexedDB({ fail = false } = {}) {
  const dbs = new Map<string, Map<string, { keyPath: string; rows: Map<unknown, Rec> }>>();
  const state: { abort: "" | "abort" | "error" } = { abort: "" };
  const later = (fn: () => void) => setTimeout(fn, 0);
  const idb = {
    state,
    open(name: string) {
      const r: Rec = {};
      later(() => {
        if (fail) {
          r.error = new Error("blocked");
          return (r.onerror as () => void)();
        }
        const fresh = !dbs.has(name);
        if (fresh) dbs.set(name, new Map());
        const stores = dbs.get(name)!;
        r.result = {
          createObjectStore: (s: string, o: { keyPath: string }) => stores.set(s, { keyPath: o.keyPath, rows: new Map() }),
          transaction(s: string) {
            const tx: Rec = { error: null };
            const st = stores.get(s)!;
            let last: Rec = {};
            const step = (fn: () => unknown) => {
              last = { result: undefined };
              const out = last;
              out.result = fn();
              later(() => {
                const end = state.abort;
                state.abort = "";
                tx.error = new Error(end || "none");
                ((end === "abort" ? tx.onabort : end === "error" ? tx.onerror : tx.oncomplete) as () => void)();
              });
              return out;
            };
            tx.objectStore = () => ({
              get: (k: unknown) => step(() => st.rows.get(k)),
              put: (v: Rec) => step(() => void st.rows.set(v[st.keyPath], structuredClone(v))),
              delete: (k: unknown) => step(() => void st.rows.delete(k)),
              getAll: () => step(() => [...st.rows.values()]),
            });
            return tx;
          },
        };
        if (fresh) (r.onupgradeneeded as () => void)?.();
        (r.onsuccess as () => void)();
      });
      return r;
    },
  };
  return idb;
}

async function exercise(store: Awaited<ReturnType<typeof openStore>>) {
  assert.equal(await store.getDraft("a"), null);
  await store.putDraft({ key: "a", path: "content/a.md", body: "x", savedAt: "2026-11-15T10:00:00.000Z" });
  await store.putDraft({ key: "new:1", path: "content/b.md", isNew: true, savedAt: "2026-11-15T10:01:00.000Z" });
  assert.equal((await store.getDraft("a"))!.body, "x");
  assert.equal((await store.listDrafts()).length, 2);
  const blob = new Blob([new Uint8Array([1, 2, 3])], { type: "image/png" });
  await store.putImage({ draft: "a", path: "m/2026/11/one-1a2b3c4d.png", blob, alt: "One" });
  await store.putImage({ draft: "a", path: "m/2026/11/two-1a2b3c4d.png", blob, alt: "Two" });
  await store.putImage({ draft: "new:1", path: "m/2026/11/one-1a2b3c4d.png", blob, alt: "Same bytes, another draft" });
  assert.deepEqual((await store.imagesOf("a")).map((i: Rec) => i.alt).sort(), ["One", "Two"]);
  assert.deepEqual(new Uint8Array(await ((await store.imagesOf("a"))[0].blob as Blob).arrayBuffer()), new Uint8Array([1, 2, 3]));
  await store.deleteImage("a", "m/2026/11/two-1a2b3c4d.png");
  assert.equal((await store.imagesOf("a")).length, 1);
  await store.deleteDraft("a");
  assert.equal(await store.getDraft("a"), null);
  assert.deepEqual(await store.imagesOf("a"), [], "a draft's images go with it");
  assert.equal((await store.imagesOf("new:1")).length, 1, "another draft's image with the same path stays");
}

test("IndexedDB: drafts and images round-trip; deleting a draft deletes its images only", async () => {
  const idb = fakeIndexedDB();
  const store = await openStore(idb as never);
  assert.equal(store.persistent, true);
  await exercise(store);
  const again = await openStore(idb as never);
  assert.equal((await again.getDraft("new:1"))!.isNew, true, "a second open finds what the first wrote");
  idb.state.abort = "abort";
  await assert.rejects(again.putDraft({ key: "z" }), /abort/, "an aborted write is a failure, not silence");
  idb.state.abort = "error";
  await assert.rejects(again.listDrafts(), /error/);
});

test("without IndexedDB, or when it refuses, the store keeps drafts in memory and says so", async () => {
  for (const idb of [undefined, fakeIndexedDB({ fail: true }), { open: () => { throw new Error("SecurityError"); } }]) {
    const store = await openStore(idb as never);
    assert.equal(store.persistent, false);
    await exercise(store);
  }
  const g = globalThis as Record<string, unknown>;
  assert.equal(g.indexedDB, undefined);
  assert.equal((await openStore()).persistent, false, "the default is the page's indexedDB");
});

test("rescue: one synchronous copy per page, read once for its own draft; newer picks the later save", () => {
  const data = new Map<string, string>();
  const storage = { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v), removeItem: (k: string) => void data.delete(k) };
  assert.equal(writeRescue(storage, { key: "a", body: "typed just before leaving", savedAt: "2" }), true);
  assert.equal(takeRescue(storage, "b"), null, "another draft's copy is not taken");
  assert.equal(takeRescue(storage, "a")!.body, "typed just before leaving");
  assert.equal(takeRescue(storage, "a"), null, "read once");
  assert.equal(writeRescue(null, { key: "a" }), false);
  assert.equal(takeRescue(null, "a"), null);
  data.set("ssg-editor-rescue", "{broken");
  assert.equal(takeRescue(storage, "a"), null);
  const a = { savedAt: "2026-11-15T10:00:00.000Z" };
  const b = { savedAt: "2026-11-15T10:00:05.000Z" };
  assert.deepEqual([newer(a, b), newer(b, a), newer(null, a), newer(a, null), newer(null, null)], [b, b, a, a, null]);
});
