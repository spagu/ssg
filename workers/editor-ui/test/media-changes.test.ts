// Images inside POST /api/changes (GO-171): one commit with the article and
// its images, editor-ui:media for every image, the type from the bytes, the
// canonical name, the plan's limits, the same image kept rather than written twice.
import { test } from "node:test";
import assert from "node:assert/strict";
import { makeEnv, site } from "./fixtures.ts";
import { AUTHOR, MEDIA, bytesAt, bytesOf, headOf, image, jpeg, png, publish, seedBytes } from "./media-fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { LIMITS } from "../src/media-change.ts";

const POST = "content/blog/posts/cats.md";

test("an article with two pasted images is one commit with three files, linking the committed paths", async () => {
  const gh = await site();
  const before = gh.store.refs.get("main")!;
  const [a, b] = [await image(png("first")), await image(jpeg("second"), "Photo 1.JPEG")];
  assert.match(b.path, /^content\/blog\/media\/2026\/11\/photo-1-[0-9a-f]{8}\.jpg$/);
  const text = `# Cats\n\n![A cat](${a.path.replace(MEDIA, "/media")})\n![A photo](${b.path.replace(MEDIA, "/media")})\n`;
  const res = await publish(gh, { message: "Cats", changes: [{ op: "put", path: POST, version: null, text }, a, b] });
  assert.equal(res.status, 201);
  const out = await res.json();
  assert.deepEqual(out.files.map((f: { path: string }) => f.path).sort(), [POST, a.path, b.path].sort());
  assert.deepEqual(headOf(gh).parents, [before], "exactly one new commit");
  const changed = [...gh.store.flatAt(out.commitId)].filter(([p, e]) => gh.store.flatAt(before).get(p)?.sha !== e.sha).map(([p]) => p);
  assert.deepEqual(changed.sort(), [POST, a.path, b.path].sort());
  assert.deepEqual(bytesAt(gh, a.path), png("first"));
  assert.deepEqual(bytesAt(gh, b.path), jpeg("second"));
  for (const p of [a.path, b.path]) assert.ok(gh.store.text(gh.store.blobIdAt("main", POST)!).includes(p.replace(MEDIA, "/media")));
});

test("without editor-ui:media a publication with an image is refused whole; the text alone passes", async () => {
  const gh = await site();
  const before = gh.store.refs.get("main");
  const writer = [PERM.read, PERM.publish];
  const text = { op: "put", path: POST, version: null, text: "# Cats\n" };
  const res = await publish(gh, { message: "Cats", changes: [text, await image(png("x"))] }, writer);
  assert.equal(res.status, 403);
  assert.deepEqual(await res.json(), { error: "forbidden", permission: PERM.media });
  assert.equal(gh.store.refs.get("main"), before, "nothing written");
  assert.equal((await publish(gh, { message: "Cats", changes: [text] }, writer)).status, 201);
  const noPublish = await publish(gh, { message: "x", changes: [await image(png("y"))] }, [PERM.read, PERM.media]);
  assert.deepEqual(await noPublish.json(), { error: "forbidden", permission: PERM.publish }, "a publication needs editor-ui:publish");
});

const refusal = async (res: Response) => { const j = await res.json(); return [res.status, j.error, j.reason]; };

test("the bytes decide: HTML or SVG named .png, unknown bytes, a name that does not match them are refused", async () => {
  const gh = await site();
  const one = async (change: unknown) => refusal(await publish(gh, { message: "x", changes: [change] }));
  const html = bytesOf("<!doctype html><script>alert(1)</script>");
  assert.deepEqual(await one(await image(html)), [400, "invalid_image", "markup"]);
  assert.deepEqual(await one(await image(bytesOf("<svg xmlns='http://www.w3.org/2000/svg' onload='alert(1)'/>"))), [400, "invalid_image", "markup"]);
  assert.deepEqual(await one(await image(bytesOf("just some text"))), [400, "invalid_image", "not_an_image"]);
  const real = await image(png("real"));
  const asGif = { ...real, path: real.path.replace(/\.png$/, ".gif") };
  const r = await publish(gh, { message: "x", changes: [asGif] });
  const body = await r.json();
  assert.deepEqual([r.status, body.reason, body.expected, body.type], [400, "type_mismatch", real.path, "png"]);
  const other = { ...real, path: real.path.replace(/-[0-9a-f]{8}\./, "-00000000.") };
  assert.deepEqual(await one(other), [400, "invalid_image", "hash_mismatch"]);
  assert.deepEqual(await one({ ...real, path: `${MEDIA}/cat.png` }), [400, "invalid_image", "not_canonical"]);
  assert.deepEqual(await one({ ...real, path: real.path.replace(MEDIA, "content/blog/posts") }), [400, "invalid_path", "outside_roots"]);
  assert.deepEqual(await one({ ...real, path: `${MEDIA}/2026/11/cat.md` }), [400, "invalid_path", "extension"]);
  assert.deepEqual(await one({ ...real, base64: "%%%%" }), [400, "invalid_change", undefined]);
  assert.equal(gh.store.refs.size, 1);
});

test("a polyglot — a valid PNG header with HTML appended — is accepted, as an image", async () => {
  const gh = await site();
  const poly = png("\r\n<html><script>alert(1)</script></html>");
  const change = await image(poly);
  assert.equal((await publish(gh, { message: "x", changes: [change] })).status, 201);
  assert.deepEqual(bytesAt(gh, change.path), poly);
});

test("malformed image changes: base64 with text, base64 not a string, a version, a move", async () => {
  const gh = await site();
  const real = await image(png("r"));
  const cases: [unknown, string][] = [
    [{ ...real, text: "x" }, "an image carries base64 only"],
    [{ ...real, base64: 7 }, "base64 must be the image's bytes, base64-encoded"],
    [{ ...real, base64: "" }, "base64 must be the image's bytes, base64-encoded"],
    [{ ...real, version: "abc" }, "an image is always new: version must be null"],
    [{ op: "move", from: real.path, to: real.path + "x", version: "abc", base64: real.base64 }, "an image is not moved; add it under its new name"],
  ];
  for (const [change, detail] of cases) {
    const res = await publish(gh, { message: "x", changes: [change] });
    assert.deepEqual(await res.json(), { error: "invalid_change", detail, index: 0 }, detail);
  }
});

test("the plan's limits: files per publication, bytes per image, bytes per publication — before anything is decoded", async () => {
  const gh = await site();
  const big = (n: number) => ({ op: "put", path: `${MEDIA}/2026/11/a-00000000.png`, version: null, base64: "A".repeat(Math.ceil((n * 4) / 3)) });
  const many = Array.from({ length: LIMITS.maxFilesPerChange + 1 }, (_, i) => ({ op: "put", path: `docs/f${i}.md`, version: null, text: "x" }));
  for (const [changes, detail] of [
    [many, `at most ${LIMITS.maxFilesPerChange} files per publication`],
    [[big(LIMITS.maxFileBytes + 3)], `an image may have at most ${LIMITS.maxFileBytes} bytes`],
    [[big(LIMITS.maxFileBytes * 0.6), big(LIMITS.maxFileBytes * 0.6)], `the images of one publication may have at most ${LIMITS.maxChangeBytes} bytes`],
  ] as const) {
    const res = await publish(gh, { message: "x", changes });
    assert.equal(res.status, 413);
    assert.deepEqual(await res.json(), { error: "too_large", detail, limits: LIMITS });
  }
});

test("the same image again is kept, not written twice: in one publication, in a later one, alone", async () => {
  const gh = await site();
  const cat = await image(png("cat"));
  const first = await publish(gh, { message: "x", changes: [cat, { ...cat }, { op: "put", path: POST, version: null, text: "a" }] });
  assert.equal(first.status, 201);
  const version = gh.store.blobIdAt("main", cat.path);
  assert.equal((await first.json()).files.length, 2, "the image once");
  const again = await publish(gh, { message: "y", changes: [{ op: "put", path: "docs/other.md", version: null, text: "b" }, cat] });
  const out = await again.json();
  assert.equal(again.status, 201);
  assert.deepEqual(out.files, [{ path: "docs/other.md", version: gh.store.blobIdAt("main", "docs/other.md") }, { path: cat.path, version }]);
  const head = gh.store.refs.get("main");
  const alone = await (await publish(gh, { message: "z", changes: [cat] })).json();
  assert.deepEqual([alone.commitId, alone.files], [head, [{ path: cat.path, version }]]);
  assert.equal(gh.store.refs.get("main"), head, "no commit");
});

test("another file at the image's path is a conflict, never overwritten", async () => {
  const gh = await site();
  const cat = await image(png("cat"));
  await seedBytes(gh, { [cat.path]: png("not the cat") });
  const res = await publish(gh, { message: "x", changes: [cat] });
  const out = await res.json();
  assert.equal(res.status, 409);
  assert.deepEqual(out.files.map((f: Record<string, unknown>) => [f.path, f.yours, f.theirsUnread]), [[cat.path, null, "not_text"]]);
});

test("deleting an image needs editor-ui:delete; an image put needs a media folder ssg publishes, text does not", async () => {
  const gh = await site();
  const old = `${MEDIA}/old.jpeg`;
  await seedBytes(gh, { [old]: jpeg("old") });
  const del = { op: "delete", path: old, version: gh.store.blobIdAt("main", old) };
  assert.deepEqual(await (await publish(gh, { message: "x", changes: [del] })).json(), { error: "forbidden", permission: PERM.delete });
  assert.equal((await publish(gh, { message: "x", changes: [del] }, [...AUTHOR, PERM.delete])).status, 201);
  assert.equal(gh.store.blobIdAt("main", old), null);
  const env = makeEnv({ EDITOR_MEDIA_DIR: "uploads" });
  const res = await publish(gh, { message: "x", changes: [await image(png("u"))] }, AUTHOR, env);
  assert.deepEqual([res.status, (await res.json()).error], [503, "media_not_configured"]);
  assert.equal((await publish(gh, { message: "x", changes: [{ op: "put", path: POST, version: null, text: "t" }] }, AUTHOR, env)).status, 201);
  const gone = await publish(gh, { message: "x", changes: [{ op: "delete", path: "content/blog/a.png", version: "abc" }] }, [...AUTHOR, PERM.delete], env);
  assert.deepEqual([gone.status, (await gone.json()).reason], [400, "media"], "no media folder: an image path is no content either");
});
