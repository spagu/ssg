// The route table of GO-140, generated from the files like GO-151's
// (go151-routes.test.ts): every handler under functions/api/auth/mfa/ and
// functions/auth/mfa/ has exactly one line here, saying who may call it; each
// line is then checked — absent (404) while the second factor is off, the
// CSRF defence on every POST/DELETE, and the refusal without the session or
// the pending step it needs.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { Browser, call, makeEnv, useClock } from "./fakes.ts";
import { mfaEnv, signIn } from "./mfa.ts";
import { seedUser } from "./seed.ts";

const API = new URL("../functions/api/auth/mfa/", import.meta.url).pathname;
const PAGES = new URL("../functions/auth/mfa/", import.meta.url).pathname;
// pending: the pending-step cookie of a sign-in; session: a signed-in session;
// owner: a session holding `*`; page: a page (GET, redirects instead of 401).
type Who = "pending" | "session" | "owner" | "page";
const ROUTES: Record<string, { who: Who; form: boolean; status?: number }> = {
  "POST api verify.ts": { who: "pending", form: true },
  "POST api break-glass.ts": { who: "pending", form: true },
  "POST api step-up.ts": { who: "session", form: true },
  "POST api backup-codes.ts": { who: "session", form: true },
  "POST api totp/start.ts": { who: "session", form: true },
  "POST api totp/confirm.ts": { who: "session", form: true },
  "GET api factors/index.ts": { who: "session", form: false },
  "POST api factors/[id].ts": { who: "session", form: true },
  "DELETE api factors/[id].ts": { who: "session", form: true },
  "POST api passkey/options.ts": { who: "session", form: false },
  "POST api passkey/register.ts": { who: "session", form: true },
  "POST api reset.ts": { who: "owner", form: false },
  "GET pages index.ts": { who: "page", form: false },
  "GET pages enrol.ts": { who: "page", form: false },
  "GET pages step-up.ts": { who: "page", form: false },
};

const files = (root: string, dir = root): string[] =>
  readdirSync(dir).flatMap((n) => (statSync(join(dir, n)).isDirectory() ? files(root, join(dir, n)) : n.startsWith("_") ? [] : [relative(root, join(dir, n))]));

const handlers = async () => {
  const out: string[] = [];
  for (const [area, root] of [["api", API], ["pages", PAGES]]) {
    for (const file of files(root)) {
      for (const name of Object.keys(await import(join(root, file)))) {
        const m = /^onRequest(Get|Post|Put|Delete|Patch)$/.exec(name);
        if (m) out.push(`${m[1].toUpperCase()} ${area} ${file}`);
      }
    }
  }
  return out.sort();
};

test("GO-140 route table: every handler under mfa/ has exactly one line, and vice versa", async () => {
  assert.deepEqual(await handlers(), Object.keys(ROUTES).sort());
});

for (const [key, spec] of Object.entries(ROUTES)) {
  const [method, area, file] = key.split(" ");
  test(`GO-140 route table: ${key} — ${spec.who}${method === "GET" ? "" : ", CSRF-checked"}`, async () => {
    useClock();
    const root = area === "api" ? API : PAGES;
    const handler = (await import(join(root, file)))[`onRequest${method[0]}${method.slice(1).toLowerCase()}`];
    const url = (area === "api" ? "/api/auth/mfa/" : "/auth/mfa/") + file.replace(/(\/)?index\.ts$/, "").replace(/\.ts$/, "").replace("[id]", "x");
    const params = { id: "x" };
    const json = file === "passkey/options.ts" ? { purpose: "register" } : {};
    const body = method === "GET" ? {} : { json };
    // The second factor off: the API is absent; a page sends the browser on.
    const off = await call(handler, makeEnv(), url, { method, params, ...body });
    if (spec.who === "page") assert.equal(off.status, 303, `${key} off`);
    else assert.ok(off.status === 404 || off.status === 401, `${key} off: ${off.status}`);
    const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: "y".repeat(40) });
    await seedUser(env, "ada@example.com");
    const { browser } = await signIn(env, "ada@example.com");
    if (method !== "GET") {
      const cross = await call(handler, env, url, { method, params, browser, json, headers: { origin: "https://evil.example.com" } });
      assert.equal(cross.status, 403, `${key} from another origin`);
      const bare = await call(handler, env, url, { method, params, browser, json, csrf: false });
      assert.equal(bare.status, 403, `${key} without the defence`);
      if (!spec.form) {
        const form = await call(handler, env, url, { method, params, browser, form: { purpose: "register" } });
        assert.equal(form.status, 403, `${key} takes no plain form`);
      }
    }
    const none = await call(handler, env, url, { method, params, browser: new Browser(), ...body });
    if (spec.who === "page") assert.equal(none.status, 303, `${key} without a session or a pending step`);
    else if (spec.who === "pending") assert.deepEqual([none.status, (await none.json()).error], [401, "mfa_pending_invalid"]);
    else assert.deepEqual([none.status, (await none.json()).error], [401, "not_authenticated"]);
    if (spec.who === "owner") {
      const notOwner = await call(handler, env, url, { method, params, browser, json: { userId: "x" } });
      assert.deepEqual(await notOwner.json(), { error: "forbidden", permission: "*" });
    }
  });
}
