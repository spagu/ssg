// The route table of GO-139, generated from the files like GO-151's
// (go151-routes.test.ts): every handler under functions/auth/access/ and
// functions/api/auth/admin/access-groups/ has exactly one line here, saying
// who may call it; each line is then checked. (admin-routes.test.ts, GO-133's
// table of every permission-checked route, needs the two admin lines too.)
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { call, makeEnv, useClock } from "./fakes.ts";
import { loggedIn, signedIn } from "./seed.ts";
import { FakeAccess, accessEnv, mint } from "./access-fake.ts";

const FUNCTIONS = new URL("../functions/", import.meta.url).pathname;
// access: Cloudflare Access's token (and, for link, a recent session); admin: auth:admin.
type Who = "access" | "access+recent" | "auth:admin";
const ROUTES: Record<string, { who: Who; csrf: boolean }> = {
  "GET auth/access/index.ts": { who: "access", csrf: false },
  "GET auth/access/link.ts": { who: "access+recent", csrf: false },
  "POST auth/access/link.ts": { who: "access+recent", csrf: true },
  "GET api/auth/admin/access-groups/index.ts": { who: "auth:admin", csrf: false },
  "POST api/auth/admin/access-groups/index.ts": { who: "auth:admin", csrf: true },
  "DELETE api/auth/admin/access-groups/[id].ts": { who: "auth:admin", csrf: true },
};

const files = (dir: string): string[] => readdirSync(dir).flatMap((n) => (statSync(join(dir, n)).isDirectory() ? files(join(dir, n)) : n.startsWith("_") ? [] : [relative(FUNCTIONS, join(dir, n))]));

test("GO-139 route table: every handler of the Access routes has exactly one line", async () => {
  const found: string[] = [];
  for (const file of files(FUNCTIONS).filter((f) => /^(auth\/access\/|api\/auth\/admin\/access-groups\/)/.test(f))) {
    for (const name of Object.keys(await import(join(FUNCTIONS, file)))) {
      const m = /^onRequest(Get|Post|Put|Delete|Patch)$/.exec(name);
      if (m) found.push(`${m[1].toUpperCase()} ${file}`);
    }
  }
  assert.deepEqual(found.sort(), Object.keys(ROUTES).sort());
});

for (const [key, spec] of Object.entries(ROUTES)) {
  const [method, file] = key.split(" ");
  test(`GO-139 route table: ${key} — ${spec.who}${spec.csrf ? ", CSRF-checked" : ""}`, async () => {
    useClock();
    const fake = new FakeAccess().install();
    try {
      const handler = (await import(join(FUNCTIONS, file)))[`onRequest${method[0]}${method.slice(1).toLowerCase()}`];
      const url = "/" + file.replace(/(\/index)?\.ts$/, "").replace("[id]", "x");
      const params = { id: "x" };
      const body = method === "GET" ? {} : spec.who === "auth:admin" ? { json: {} } : { form: {} };
      const env = accessEnv();
      if (spec.csrf) {
        const { browser } = spec.who === "auth:admin" ? await signedIn(env, "admin@example.com", ["auth:admin"]) : await loggedIn(env);
        const cross = await call(handler, env, url, { method, params, browser, ...body, headers: { origin: "https://evil.example.com" } });
        assert.equal(cross.status, 403, `${key} from another origin`);
      }
      if (spec.who === "auth:admin") {
        const none = await call(handler, env, url, { method, params, ...body });
        assert.deepEqual([none.status, await none.json()], [401, { error: "not_authenticated" }]);
        const { browser } = await signedIn(env, "reader@example.com", ["newsletter:read"]);
        const without = await call(handler, env, url, { method, params, browser, ...body });
        assert.deepEqual([without.status, await without.json()], [403, { error: "forbidden", permission: "auth:admin" }]);
        return;
      }
      // Access routes: without Access configured they do not exist.
      assert.equal((await call(handler, makeEnv(), url, { method, ...body })).status, 404);
      // Configured, with no Access token: refused, never signed in or linked.
      const { browser } = await loggedIn(env, "member@example.com");
      const res = await call(handler, env, url, { method, browser, ...body });
      assert.equal(res.status, 303);
      assert.match(res.headers.get("location") as string, /error=access_failed/);
      if (spec.who === "access+recent") {
        // Without a session: to sign in first, the token is not even looked at.
        const anon = await call(handler, env, url, { method, ...body, headers: { "cf-access-jwt-assertion": await mint() } });
        assert.equal(anon.status, 303);
        assert.match(anon.headers.get("location") as string, /^\/(api\/auth\/resume|auth\/login)\?/);
      }
    } finally {
      fake.restore();
    }
  });
}
