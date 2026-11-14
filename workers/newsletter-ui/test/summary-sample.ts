// The summary sample workers/modules.test.ts reads (GO-148): drives GET
// {base}/api/module/summary through the whole fetch pipeline with the test
// fakes, on a database where every attention item shows, and prints the body.
// Run from the repository root:
//   node --import ./workers/newsletter-ui/test/resolve.mjs workers/newsletter-ui/test/summary-sample.ts
//
// SUMMARY_DB=<file> seeds that SQLite file instead of an in-memory database,
// so the newsletter template's sample can read the same subscribers.
import { DatabaseSync } from "node:sqlite";
import { handleFetch } from "../src/app.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { seededEnv } from "./summary-fixtures.ts";

const file = process.env.SUMMARY_DB;
const env = await seededEnv({ NEWSLETTER_SITE_URL: undefined }, file ? new DatabaseSync(file) : undefined);
const res = await handleFetch(req("/newsletter-admin/api/module/summary", { auth: PASSWORD }), env as never, fakeCtx() as never);
const body = await res.text();
if (res.status !== 200) throw new Error(`summary answered ${res.status}: ${body}`);
process.stdout.write(body);
