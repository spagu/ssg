// The summary sample workers/modules.test.ts reads: drives GET
// {base}/api/module/summary through the whole fetch pipeline — the gate with
// a signed session, the fake GitHub in-process — on a site where every kind
// of item shows (two metrics, the branch, a problem), and prints the body.
// Run from the repository root:
//   node --import ./workers/designer-ui/test/resolve.mjs workers/designer-ui/test/summary-sample.ts
import { BASE, makeEnv, request, run, site } from "./fixtures.ts";

// The settings file holds a key ssg does not know: the build would stop.
const gh = await site({ ".ssg.designer.json": '{"title": "Changed", "no_such_key": 1}\n' });
const res = await run(await request(BASE + "/api/module/summary", ["designer-ui:read"]), makeEnv(), gh);
const body = await res.text();
if (res.status !== 200) throw new Error(`summary answered ${res.status}: ${body}`);
process.stdout.write(body);
