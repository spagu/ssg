// The route table of GO-151, generated from the files like GO-134's
// (go134-routes.test.ts): every handler in the GO-151 route files has exactly
// one line here, saying who may call it; each line is then checked.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { call, makeEnv, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import { providerEnv } from "./go151.ts";

const ROOT = new URL("../functions/api/auth/", import.meta.url).pathname;
// public: no session needed (a provider's own guard: the flow cookie and state);
// recent: a session started within AUTH_RECENT_SIGNIN_MINUTES; session: any.
type Who = "public" | "session" | "recent";
const ROUTES: Record<string, { who: Who; csrf: boolean; provider?: boolean }> = {
  "POST oauth/[provider]/start.ts": { who: "public", csrf: true, provider: true },
  "GET oauth/[provider]/callback.ts": { who: "public", csrf: false, provider: true },
  // Apple's cross-site form_post: no CSRF defence by design — it only bounces to the GET.
  "POST oauth/[provider]/callback.ts": { who: "public", csrf: false, provider: true },
  "GET methods/index.ts": { who: "session", csrf: false },
  "POST methods/set-password.ts": { who: "recent", csrf: true },
  "POST methods/[id].ts": { who: "recent", csrf: true },
  "DELETE methods/[id].ts": { who: "recent", csrf: true },
};

const files = (dir: string): string[] =>
  readdirSync(dir).flatMap((n) => (statSync(join(dir, n)).isDirectory() ? files(join(dir, n)) : n.startsWith("_") ? [] : [relative(ROOT, join(dir, n))]));

test("GO-151 route table: every handler under oauth/ and methods/ has exactly one line", async () => {
  const found: string[] = [];
  for (const file of files(ROOT).filter((f) => /^(oauth|methods)\//.test(f))) {
    for (const name of Object.keys(await import(join(ROOT, file)))) {
      const m = /^onRequest(Get|Post|Put|Delete|Patch)$/.exec(name);
      if (m) found.push(`${m[1].toUpperCase()} ${file}`);
    }
  }
  assert.deepEqual(found.sort(), Object.keys(ROUTES).sort());
});

for (const [key, spec] of Object.entries(ROUTES)) {
  const [method, file] = key.split(" ");
  test(`GO-151 route table: ${key} — ${spec.who}${spec.csrf ? ", CSRF-checked" : ""}`, async () => {
    useClock();
    const handler = (await import(join(ROOT, file)))[`onRequest${method[0]}${method.slice(1).toLowerCase()}`];
    const params: Record<string, string> = spec.provider ? { provider: "google" } : { id: "password" };
    const url = "/api/auth/" + file.replace(/(\/index)?\.ts$/, "").replace("[provider]", "google").replace("[id]", "password");
    if (spec.provider) {
      // Without the provider's variables the route does not exist.
      const res = await call(handler, makeEnv(), url, { method, params, ...(method === "GET" ? {} : { form: { a: "b" } }) });
      assert.equal(res.status, 404);
    }
    const env = providerEnv();
    if (spec.csrf) {
      const { browser } = await loggedIn(env);
      const cross = await call(handler, env, url, { method, params, browser, json: {}, headers: { origin: "https://evil.example.com" } });
      assert.equal(cross.status, 403, `${key} from another origin`);
      const bare = await call(handler, env, url, { method, params, browser, json: {}, csrf: false });
      assert.equal(bare.status, 403, `${key} without the defence`);
    }
    if (spec.who !== "public") {
      const none = await call(handler, env, url, { method, params, ...(method === "GET" ? {} : { json: {} }) });
      assert.deepEqual([none.status, await none.json()], [401, { error: "not_authenticated" }]);
    }
  });
}
