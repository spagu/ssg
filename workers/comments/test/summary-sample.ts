// The summary sample workers/modules.test.ts reads (GO-148): drives GET
// /api/comments/module/summary through the test fakes, on a database where
// every attention item and a problem show, and prints the body. Run from the
// repository root:
//   node --import ./workers/comments/test/resolve.mjs workers/comments/test/summary-sample.ts
import * as route from "../functions/api/comments/module/summary.ts";
import { PASSWORD, basic, call, seed, sqliteD1 } from "./harness.ts";

const db = sqliteD1();
const ago = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString();
seed(db, [
  { id: "p1", status: "pending", created_at: ago(30) },
  { id: "p2", status: "pending", created_at: ago(0) },
  { id: "a1", status: "approved", created_at: ago(5) },
  { id: "s1", status: "spam", created_at: ago(2) },
]);
// No Turnstile secret: the summary reports it as a problem.
const got = await call(route.onRequestGet, { path: "/api/comments/module/summary", headers: { authorization: basic(PASSWORD) }, env: { COMMENTS_DB: db, COMMENTS_ADMIN_PASSWORD: PASSWORD } });
if (got.status !== 200) throw new Error(`summary answered ${got.status}: ${got.body}`);
process.stdout.write(got.body);
