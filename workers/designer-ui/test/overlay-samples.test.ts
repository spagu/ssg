// Settings files this worker writes, kept as a fixture for ssg: a Go test
// reads test/overlay-samples.json and runs each `text` through
// config.ValidateOverlay (and the loader), so a file the panel publishes can
// never stop the site's build (ticket GO-178, acceptance criteria). The
// samples are produced here, through the whole pipeline; this test fails while
// the fixture differs from what the worker writes. Regenerate it with
//
//   UPDATE_OVERLAY_SAMPLES=1 node --import ./workers/designer-ui/test/resolve.mjs --test workers/designer-ui/test/overlay-samples.test.ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import { ADVANCED, site } from "./fixtures.ts";
import { fileAt, publish } from "./helpers.ts";
import { validateOverlay } from "../src/overlay-validate.ts";

const FIXTURE = new URL("./overlay-samples.json", import.meta.url);

// Each sample is a sequence of publications on a fresh site; the file after
// the last one is the sample.
const CASES: { name: string; steps: { path: string[]; value: unknown }[][] }[] = [
  { name: "plain keys at the top level", steps: [[{ path: ["title"], value: "Example" }, { path: ["description"], value: "A site" }, { path: ["port"], value: 8080 }, { path: ["minify_html"], value: true }]] },
  { name: "nested object fields and an enum", steps: [[{ path: ["mddb", "lang"], value: "pl_PL" }, { path: ["mddb", "protocol"], value: "grpc" }, { path: ["mddb", "timeout"], value: 10 }]] },
  { name: "map entries, a key holding a dot", steps: [[{ path: ["colors", "primary"], value: "#00695c" }, { path: ["ai", "models", "gpt-4.1"], value: { url: "https://example.com/v1/chat", key: "$AI_KEY" } }]] },
  { name: "lists replaced whole, a list of objects", steps: [[{ path: ["languages"], value: [{ code: "en", locale: "en_US", name: "English" }, { code: "pl", locale: "pl_PL", name: "Polski" }] }, { path: ["hooks", "pre_build"], value: ["npm ci", "npm run build"] }]] },
  { name: "secrets as references in every form", steps: [[{ path: ["jwt_secret"], value: "${JWT_SECRET}" }, { path: ["server_users"], value: ["admin:$ADMIN_PASSWORD"] }, { path: ["mddb", "api_key"], value: "$MDDB_KEY" }, { path: ["notifications"], value: [{ name: "chat", url: "https://example.com/hook", headers: { Authorization: "Bearer $HOOK_TOKEN" } }] }]] },
  { name: "an alternate form and an open object", steps: [[{ path: ["story"], value: true }, { path: ["pretty_urls"], value: false }], [{ path: ["story"], value: { enabled: true, sections: "posts", custom_flag: 1 } }]] },
  { name: "removals prune emptied objects", steps: [[{ path: ["mddb", "lang"], value: "en" }, { path: ["title"], value: "T" }, { path: ["variables", "brand"], value: { name: "Acme" } }], [{ path: ["mddb", "lang"], value: null }]] },
];

async function produce(): Promise<{ name: string; text: string }[]> {
  const out: { name: string; text: string }[] = [];
  for (const c of CASES) {
    const gh = await site();
    for (const changes of c.steps) {
      const r = await publish(gh, ADVANCED, changes);
      assert.equal(r.status, 200, `${c.name}: ${r.text}`);
    }
    out.push({ name: c.name, text: fileAt(gh, ".ssg.designer.json")! });
  }
  return out;
}

test("the settings files the worker writes are the fixture ssg validates, and pass the worker's own validation", async () => {
  const samples = await produce();
  for (const s of samples) {
    assert.deepEqual(validateOverlay(JSON.parse(s.text)), [], s.name);
    assert.ok(s.text.endsWith("}\n"), s.name);
  }
  const text = JSON.stringify(samples, null, 2) + "\n";
  if (process.env.UPDATE_OVERLAY_SAMPLES === "1") writeFileSync(FIXTURE, text);
  assert.equal(readFileSync(FIXTURE, "utf8"), text, "test/overlay-samples.json differs from what the worker writes — regenerate it (see the top of this file)");
});
