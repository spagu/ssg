// portable/config-files.ts names the overlay file for both panel workers
// (GO-185); the configuration schema generated from ssg names it too. The two
// must agree, or the worker would write a file ssg never merges.
import { test } from "node:test";
import assert from "node:assert/strict";
import { OVERLAY_FILE } from "../portable/config-files.ts";
import { SCHEMA } from "../src/schema.ts";

test("the shared overlay file name is the one ssg's schema names", () => {
  assert.equal(OVERLAY_FILE, SCHEMA.overlayFile);
});
