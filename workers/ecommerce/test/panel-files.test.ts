// Files the panel saves — the orders CSV and an invoice — fetched with the
// panel's own credentials (api.file, files.js), and the payments screen's
// `backticked` commands drawn as <code> (dom.js withCode). A plain link lost
// the shared password, which lives in the panel's memory only, so the export
// was a 401 and the browser cancelled the download.
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as client from "../public/ecommerce-admin/api.js";
import { badge, clearNotice, confirmDestructive, el, emptyRow, notify, td, titleCell, withCode } from "../public/ecommerce-admin/dom.js";
import { bindDownload, download, fileName, saveBlob } from "../public/ecommerce-admin/files.js";

const realFetch = globalThis.fetch;
const realDocument = (globalThis as { document?: unknown }).document;
const realCreate = URL.createObjectURL;
const realRevoke = URL.revokeObjectURL;

/** The few DOM calls these modules make, recorded. */
class FakeNode {
  children: unknown[] = [];
  attrs: Record<string, string> = {};
  listeners: Record<string, (e: unknown) => void> = {};
  dataset: Record<string, string> = {};
  className = "";
  textContent = "";
  hidden = false;
  href = "";
  download = "";
  clicked = 0;
  removed = false;
  tag: string;
  constructor(tag: string) { this.tag = tag; }
  setAttribute(k: string, v: string) { this.attrs[k] = v; }
  getAttribute(k: string) { return this.attrs[k] ?? null; }
  addEventListener(type: string, fn: (e: unknown) => void) { this.listeners[type] = fn; }
  append(...nodes: unknown[]) { this.children.push(...nodes); }
  click() { this.clicked++; }
  remove() { this.removed = true; }
}
let created: FakeNode[] = [];
const realNode = (globalThis as { Node?: unknown }).Node;
const realWindow = (globalThis as { window?: unknown }).window;
let notice: FakeNode;
let revoked: string[] = [];

beforeEach(() => {
  created = [];
  revoked = [];
  notice = new FakeNode("div");
  (globalThis as { Node?: unknown }).Node = FakeNode;
  (globalThis as { document?: unknown }).document = {
    createElement: (tag: string) => {
      const node = new FakeNode(tag);
      created.push(node);
      return node;
    },
    createTextNode: (text: string) => text,
    body: new FakeNode("body"),
    getElementById: (id: string) => (id === "notice" ? notice : null),
  };
  URL.createObjectURL = (() => "blob:https://shop.example.com/1") as typeof URL.createObjectURL;
  URL.revokeObjectURL = ((url: string) => void revoked.push(url)) as typeof URL.revokeObjectURL;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  (globalThis as { document?: unknown }).document = realDocument;
  (globalThis as { Node?: unknown }).Node = realNode;
  (globalThis as { window?: unknown }).window = realWindow;
  URL.createObjectURL = realCreate;
  URL.revokeObjectURL = realRevoke;
  client.configure({ mode: "password" });
  client.setPassword("");
});

type Seen = { url: string; init: RequestInit & { headers: Record<string, string> } };
function serve(...answers: Response[]): Seen[] {
  const seen: Seen[] = [];
  globalThis.fetch = (async (url: string, init: Seen["init"]) => {
    seen.push({ url, init });
    return answers.shift()!;
  }) as never;
  return seen;
}
const tick = () => new Promise((r) => setTimeout(r, 1));

describe("api.file", () => {
  it("sends the shared password as Basic and returns the body as a Blob", async () => {
    client.setPassword("secret");
    const seen = serve(new Response("number,status\r\n", { headers: { "content-type": "text/csv" } }));
    const blob = await client.api.file("/api/shop/admin/export?type=orders");
    assert.equal(await blob.text(), "number,status\r\n");
    assert.equal(seen[0]!.init.headers.authorization, `Basic ${Buffer.from(":secret").toString("base64")}`);
    assert.equal(seen[0]!.init.headers.accept, "*/*");
    assert.equal(seen[0]!.init.credentials, "same-origin");
  });

  it("auth mode: no Basic header, one refresh and one retry on a 401", async () => {
    client.configure({ mode: "auth", refreshUrl: "/api/auth/refresh" });
    const seen = serve(new Response("", { status: 401 }), new Response("", { status: 200 }), new Response("ok"));
    assert.equal(await (await client.api.file("/f")).text(), "ok");
    assert.equal(seen[0]!.init.headers.authorization, undefined);
    assert.equal(seen[1]!.url, "/api/auth/refresh");
    assert.equal(seen[2]!.url, "/f");
  });

  it("a refusal is the API's error: the 403's permission, a 401 that signs out, a bare status", async () => {
    let told: unknown = "untouched";
    client.setSignedOutHandler((body: unknown) => (told = body));
    serve(
      new Response(JSON.stringify({ error: "forbidden", permission: "ecommerce:export", message: "needs ecommerce:export" }), { status: 403 }),
      new Response("", { status: 401 }),
      new Response("", { status: 500 }),
    );
    await assert.rejects(client.api.file("/f"), (e: client.ApiError) => e.permission === "ecommerce:export" && e.status === 403);
    await assert.rejects(client.api.file("/f"), (e: client.ApiError) => e.code === "unauthenticated");
    assert.equal(told, null);
    await assert.rejects(client.api.file("/f"), (e: client.ApiError) => e.code === "http_error" && /500/.test(e.message));
  });
});

describe("files.js", () => {
  it("fileName makes an invoice number safe to save", () => {
    assert.equal(fileName("FV/2026/000042", "html"), "FV-2026-000042.html");
    assert.equal(fileName("//", "csv"), "download.csv");
  });

  it("saveBlob downloads through a temporary link and revokes its URL", async () => {
    saveBlob(new Blob(["x"]), "a.csv");
    const link = created[0]!;
    assert.deepEqual([link.href, link.download, link.hidden, link.clicked, link.removed], ["blob:https://shop.example.com/1", "a.csv", true, 1, true]);
    await tick();
    assert.deepEqual(revoked, ["blob:https://shop.example.com/1"]);
  });

  it("bindDownload keeps the href, stops the navigation and saves what the fetch returned", async () => {
    client.setPassword("secret");
    const seen = serve(new Response("<html>FV</html>"));
    const link = new FakeNode("a");
    link.setAttribute("href", "/api/shop/admin/invoices/FV%2F2026%2F000001");
    assert.equal(bindDownload(link as never, "FV-2026-000001.html"), link);
    let prevented = false;
    link.listeners.click!({ preventDefault: () => (prevented = true) });
    await tick();
    assert.ok(prevented);
    assert.equal(seen[0]!.url, "/api/shop/admin/invoices/FV%2F2026%2F000001");
    assert.equal(created.at(-1)!.download, "FV-2026-000001.html");
  });

  it("download never rejects: a failure becomes the panel's notice", async () => {
    serve(new Response(JSON.stringify({ error: "not_found", message: "No such invoice." }), { status: 404 }));
    await download("/x", "x.html");
    assert.equal(notice.textContent, "No such invoice.");
    assert.equal(notice.dataset.kind, "error");
  });

  it("the export button is a link the panel binds, needing ecommerce:export", () => {
    const html = readFileSync(new URL("../public/ecommerce-admin/index.html", import.meta.url), "utf8");
    assert.match(html, /id="export-orders" href="\/api\/shop\/admin\/export\?type=orders" download data-needs="ecommerce:export"/);
    const orders = readFileSync(new URL("../public/ecommerce-admin/orders.js", import.meta.url), "utf8");
    assert.match(orders, /bindDownload\(document\.getElementById\("export-orders"\)/);
    assert.match(orders, /\/api\/shop\/admin\/invoices\/\$\{encodeURIComponent\(inv\.number\)\}/);
  });
});

describe("withCode", () => {
  it("draws backticked spans as <code> text and leaves no backtick behind", () => {
    const parts = withCode("Keys are set with `wrangler pages secret put` and never stored.") as Array<string | FakeNode>;
    assert.equal(parts.length, 3);
    assert.equal(parts[0], "Keys are set with ");
    assert.equal((parts[1] as FakeNode).tag, "code");
    assert.equal((parts[1] as FakeNode).textContent, "wrangler pages secret put");
    assert.equal(parts[2], " and never stored.");
    assert.deepEqual(withCode(undefined), []);
    assert.deepEqual(withCode("plain"), ["plain"]);
    const markup = withCode("`<b>x</b>`") as FakeNode[];
    assert.equal(markup[0]!.textContent, "<b>x</b>", "never HTML");
  });
});

describe("dom.js", () => {
  it("el: attributes, text, data, listeners and children; empty values skipped", () => {
    let clicked = 0;
    const child = new FakeNode("i");
    const node = el("button", { class: "small", text: "Go", dataset: { id: "7" }, disabled: true, hidden: false, title: null, onClick: () => clicked++ }, child, "tail", null, [false, undefined]) as unknown as FakeNode;
    assert.equal(node.className, "small");
    assert.equal(node.textContent, "Go");
    assert.equal(node.dataset.id, "7");
    assert.equal(node.attrs.disabled, "");
    assert.equal("hidden" in node.attrs || "title" in node.attrs, false);
    node.listeners.click!({});
    assert.equal(clicked, 1);
    assert.deepEqual(node.children, [child, "tail"]);
  });

  it("td carries its column name, with or without attributes; emptyRow and titleCell", () => {
    const plain = td("Qty", "1") as unknown as FakeNode;
    assert.equal(plain.attrs["data-label"], "Qty");
    assert.deepEqual(plain.children, ["1"]);
    const money = td("Total", { class: "numeric" }, "24.00") as unknown as FakeNode;
    assert.equal(money.attrs.class, undefined);
    assert.equal(money.className, "numeric");
    assert.deepEqual(money.children, ["24.00"]);
    const row = emptyRow(3, "Nothing yet") as unknown as FakeNode;
    const cell = row.children[0] as FakeNode;
    assert.deepEqual([cell.attrs.colspan, cell.textContent], ["3", "Nothing yet"]);
    const titled = titleCell("Product", "A book", "EBOOK-1") as unknown as FakeNode;
    assert.equal(((titled.children[0] as FakeNode).children.length), 2);
    const bare = titleCell("Product", "A book", "") as unknown as FakeNode;
    assert.equal(((bare.children[0] as FakeNode).children.length), 1);
    assert.equal((badge("needs_review") as unknown as FakeNode).textContent, "needs review");
  });

  it("notify shows a message, hides a non-error later, and clearNotice hides it; confirm is the browser's", async () => {
    notify("error", "Broken");
    assert.deepEqual([notice.textContent, notice.hidden, notice.attrs.role], ["Broken", false, "alert"]);
    clearNotice();
    assert.equal(notice.hidden, true);
    const realTimeout = globalThis.setTimeout;
    globalThis.setTimeout = ((fn: () => void) => { fn(); return 0; }) as never;
    try {
      notify("ok", "Saved");
    } finally {
      globalThis.setTimeout = realTimeout;
    }
    assert.deepEqual([notice.attrs.role, notice.hidden], ["status", true]);
    (globalThis as { window?: unknown }).window = { confirm: (q: string) => q === "Sure?" };
    assert.equal(confirmDestructive("Sure?"), true);
  });
});
