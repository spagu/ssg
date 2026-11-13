// src/confirm-strings.ts carries the `newsletter` template's confirmation mail
// (mailSubject, mailBody) word for word, so a pending subscriber gets the same
// mail from either worker. This fails when the copies drift. In a scaffolded
// copy without workers/newsletter next to it, it is skipped with a reason.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { CONFIRM_STRINGS } from "../src/confirm-strings.ts";

const ORIGINAL = new URL("../../newsletter/functions/api/newsletter/_i18n.ts", import.meta.url);
const skip = existsSync(ORIGINAL) ? false : "workers/newsletter is not next to this copy (scaffolded alone): confirmation strings not compared";

test("the confirmation mail strings equal the newsletter template's, for every language", { skip }, async () => {
  const { STRINGS } = (await import(ORIGINAL.href)) as { STRINGS: Record<string, { mailSubject: string; mailBody: string }> };
  assert.deepEqual(Object.keys(CONFIRM_STRINGS).sort(), Object.keys(STRINGS).sort(), "the same languages");
  for (const [lang, t] of Object.entries(STRINGS)) {
    assert.deepEqual(CONFIRM_STRINGS[lang], { mailSubject: t.mailSubject, mailBody: t.mailBody }, `${lang}: copy _i18n.ts mailSubject/mailBody again`);
  }
});
