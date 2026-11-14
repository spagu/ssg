// The summary sample workers/modules.test.ts reads (GO-148): drives GET
// /api/newsletter/module/summary through the test fakes and prints the body.
// Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs workers/newsletter/test/summary-sample.ts
//
// SUMMARY_DB=<file> reads a SQLite file another template's sample has already
// filled (newsletter-ui's: the two templates share the database) instead of
// seeding its own in-memory one.
import { DatabaseSync } from "node:sqlite";
import * as route from "../functions/api/newsletter/module/summary.ts";
import { sqliteD1, withSchemaV1 } from "./sqlite-d1.ts";
import { seedPeople } from "./summary-seed.ts";

const shared = process.env.SUMMARY_DB;
const db = shared ? sqliteD1(new DatabaseSync(shared)) : withSchemaV1(sqliteD1());
if (!shared) for (const [status, n] of [["confirmed", 3], ["pending", 2], ["unsubscribed", 1]] as const) seedPeople(db.sqlite, status, n);
const env = { NEWSLETTER_DB: db, NEWSLETTER_ADMIN_PASSWORD: "pw", NEWSLETTER_TURNSTILE_SECRET: "ts" };
const request = new Request("https://example.com/api/newsletter/module/summary", { headers: { authorization: "Basic " + btoa("admin:pw") } });
const res = await (route.onRequestGet as unknown as (c: { request: Request; env: unknown }) => Promise<Response>)({ request, env });
const body = await res.text();
if (res.status !== 200) throw new Error(`summary answered ${res.status}: ${body}`);
process.stdout.write(body);
