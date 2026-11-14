// One-time tokens are never kept or logged in clear (GO-134): a full run —
// invitations by link and by mail, a refused mail, acceptance, admin reset
// links, "forgot password", a reset — then every table, every log line and
// every answer header is scanned for every token handed out.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as resetLink } from "../functions/api/auth/admin/users/[id]/reset-link.ts";
import { onRequestPost as forgot } from "../functions/api/auth/forgot.ts";
import { onRequestPost as reset } from "../functions/api/auth/reset.ts";
import { Browser, call, useClock } from "./fakes.ts";
import { acceptWith, captureMail, invite, linkIn, rows, tokenOf, withMail } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";

test("tokens: only hashes in the database, nothing in the logs or headers", async () => {
  useClock();
  const env = withMail();
  const mail = captureMail();
  const logged: string[] = [];
  const saved = { log: console.log, warn: console.warn, error: console.error, info: console.info };
  console.log = console.warn = console.error = console.info = (...a: unknown[]) => void logged.push(a.map(String).join(" "));
  const headers: string[] = [];
  const keep = (res: Response) => (headers.push(JSON.stringify([...res.headers].filter(([k]) => k !== "set-cookie"))), res);
  const tokens: string[] = [];
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const byLink = await invite(env, boss.browser, { email: "a@example.com", delivery: "link" });
    tokens.push(tokenOf(byLink.body.link));
    await invite(env, boss.browser, { email: "b@example.com" });
    tokens.push(tokenOf(linkIn(mail.sent[0].text)));
    mail.status = 500;
    await invite(env, boss.browser, { email: "c@example.com" });
    tokens.push(tokenOf(linkIn(mail.sent[1].text)));
    mail.status = 200;
    await acceptWith(env, tokens[0]);
    const ada = await seedUser(env, "ada@example.com");
    const link = await (await call(resetLink, env, `/api/auth/admin/users/${ada}/reset-link`, { method: "POST", browser: boss.browser, params: { id: ada }, json: { delivery: "link" } })).json();
    tokens.push(tokenOf(link.link));
    keep(await call(forgot, env, "/api/auth/forgot", { json: { email: "ada@example.com" }, browser: new Browser("192.0.2.1") }));
    tokens.push(tokenOf(linkIn(mail.sent[2].text)));
    keep(await call(reset, env, "/api/auth/reset", { json: { token: tokens.at(-1), password: "a brand new passphrase" } }));
  } finally {
    Object.assign(console, saved);
    mail.restore();
  }
  assert.equal(new Set(tokens).size, 5);
  const db = ["invitations", "password_resets", "users", "sessions", "login_attempts", "grants"].map((t) => JSON.stringify(rows(env, t))).join("\n");
  for (const t of tokens) {
    assert.ok(!db.includes(t), "a token in the database");
    assert.ok(!logged.join("\n").includes(t), "a token in a log line");
    assert.ok(!headers.join("\n").includes(t), "a token in a header");
  }
  assert.ok(logged.some((l) => l.includes("mail not sent")), "the refused mail was logged — without its link");
});
