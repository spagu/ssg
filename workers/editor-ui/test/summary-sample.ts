// The summary sample workers/modules.test.ts reads: drives GET
// {base}/api/module/summary through the whole fetch pipeline — the gate with
// a signed session, the fake GitHub in-process — on a site where every kind
// of item shows (a metric, the branch, a configuration problem), and prints
// the body. Run from the repository root:
//   node --import ./workers/editor-ui/test/resolve.mjs workers/editor-ui/test/summary-sample.ts
import { BASE, makeEnv, request, run, site } from "./fixtures.ts";

// One content_sources entry points outside the repository: a "partial" problem.
const gh = await site({ ".ssg.yaml": "source: blog\ncontent_sources:\n  - path: ../outside\n" });
const res = await run(await request(BASE + "/api/module/summary", ["editor-ui:read"]), makeEnv(), gh);
const body = await res.text();
if (res.status !== 200) throw new Error(`summary answered ${res.status}: ${body}`);
process.stdout.write(body);
