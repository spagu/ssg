// The summary sample workers/modules.test.ts reads (GO-148): GET
// /api/auth/module/summary on a database where every attention item and a
// problem show, printed. Run from the repository root:
//   node --import ./workers/auth/test/resolve.mjs workers/auth/test/summary-sample.ts
import { summaryAnswer } from "./summary-fixture.ts";

const res = await summaryAnswer();
const body = await res.text();
if (res.status !== 200) throw new Error(`summary answered ${res.status}: ${body}`);
process.stdout.write(body);
