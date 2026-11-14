// GO-141: a service token's secret exists once — in the answer that creates
// it. Not in any table, not in any log line, not in any other answer; nor is
// an access token it was exchanged for stored anywhere.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as getAccount } from "../functions/api/auth/admin/service-accounts/[id]/index.ts";
import { onRequestGet as listTokens } from "../functions/api/auth/admin/service-accounts/[id]/tokens/index.ts";
import { onRequestDelete as revoke } from "../functions/api/auth/admin/service-accounts/[id]/tokens/[tokenId].ts";
import { sha256hex } from "../functions/api/auth/_lib.ts";
import { parseSecret } from "../functions/api/auth/_service-tokens.ts";
import { call, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { account, newToken, swap } from "./service.ts";

test("secrets: nowhere in the database, the logs or later answers; only their SHA-256 is kept", async (t) => {
  const logs: string[] = [];
  for (const level of ["log", "info", "warn", "error", "debug"] as const) t.mock.method(console, level, (...a: unknown[]) => logs.push(a.map(String).join(" ")));
  useClock();
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, ["web-push:notify"]);
  const created = await newToken(env, owner.browser, id, { name: "ci" });
  const secret = created.body.secret as string;
  const random = secret.slice(-43);
  const tokenId = (parseSecret(secret) as { id: string }).id;

  const answers: string[] = [];
  const keep = async (r: Response) => (answers.push(await r.clone().text()), r);
  const jwt = (await (await swap(env, `Bearer ${secret}`)).json()).accessToken as string; // its own answer, the one place it belongs
  await keep(await swap(env, `Bearer ${secret.slice(0, -1)}x`));
  await keep(await call(listTokens, env, "/x", { browser: owner.browser, params: { id } }));
  await keep(await call(getAccount, env, "/x", { browser: owner.browser, params: { id } }));
  await keep(await call(revoke, env, "/x", { method: "DELETE", browser: owner.browser, params: { id, tokenId } }));
  await keep(await swap(env, `Bearer ${secret}`));

  const db = env.AUTH_DB.sqlite;
  const tables = (db.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all() as { name: string }[]).map((r) => r.name);
  const dump = tables.map((name) => JSON.stringify(db.prepare(`SELECT * FROM "${name}"`).all())).join("\n");
  for (const [what, haystack] of [["database", dump], ["logs", logs.join("\n")], ["later answers", answers.join("\n")]] as const) {
    assert.ok(!haystack.includes(random), `the secret's random part is in the ${what}`);
    assert.ok(!haystack.includes(jwt.split(".")[2]), `the access token is in the ${what}`);
  }
  assert.ok(dump.includes(await sha256hex(secret)), "the digest is what is kept");
  assert.equal(JSON.stringify(created.body).split(random).length - 1, 1, "the creation answer holds it exactly once");
});
