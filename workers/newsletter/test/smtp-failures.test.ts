// The SMTP client when things go wrong: 4xx, 5xx, drops, timeouts, junk.
// Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { openSmtp } from "../functions/api/newsletter/_smtp.ts";
import { smtpServer } from "./fakes.ts";
import type { Stage } from "./fakes.ts";
import { cfg, msg, sendOne } from "./smtpcase.ts";

const openStages: Stage[] = ["greeting", "EHLO", "STARTTLS", "AUTH"];
const sendStages: Stage[] = ["MAIL", "RCPT", "DATA", "DOT"];

test("4xx at each stage is temporary and names the code", async () => {
  for (const stage of [...openStages, ...sendStages]) {
    const { opened, sent } = await sendOne({ on: (st) => (st === stage ? "451 4.3.0 try again later" : undefined) });
    const r = sent ?? opened;
    assert.ok(!r.ok && r.temporary, stage);
    assert.ok(!r.ok && r.detail.startsWith("smtp 451 at "), `${stage}: ${!r.ok && r.detail}`);
  }
});

test("5xx at each stage is permanent and names the code", async () => {
  for (const stage of [...openStages, ...sendStages]) {
    const { opened, sent } = await sendOne({ on: (st) => (st === stage ? "554 5.7.1 rejected" : undefined) });
    const r = sent ?? opened;
    assert.ok(!r.ok && !r.temporary && r.detail.includes("554"), stage);
  }
});

test("an unexpected code from a confused server is temporary", async () => {
  const { sent } = await sendOne({ on: (st) => (st === "MAIL" ? "354 what" : undefined) });
  assert.ok(sent && !sent.ok && sent.temporary && /^smtp 354 at MAIL FROM/.test(sent.detail));
});

test("a refused message resets the transaction and the next one goes", async () => {
  const s = smtpServer({ on: (st, line) => (st === "RCPT" && line.includes("bad@") ? "550 5.1.1 no such user" : undefined) });
  const opened = await openSmtp(cfg(), s.connect);
  assert.ok(opened.ok);
  const bad = await opened.session.send(msg({ to: "bad@example.com" }));
  assert.ok(!bad.ok && !bad.temporary && bad.detail === "smtp 550 at RCPT TO: 5.1.1 no such user");
  assert.deepEqual(await opened.session.send(msg()), { ok: true });
  await opened.session.close();
  assert.ok(s.state.log.some((l) => l.line === "RSET"));
  assert.equal(s.state.messages.length, 1);
});

test("a failed RSET ends the session", async () => {
  const s = smtpServer({ on: (st) => (st === "RCPT" ? "550 no" : st === "RSET" ? "421 closing" : undefined) });
  const opened = await openSmtp(cfg(), s.connect);
  assert.ok(opened.ok);
  assert.equal((await opened.session.send(msg())).ok, false);
  const next = await opened.session.send(msg());
  assert.ok(!next.ok && next.temporary && /session is closed/.test(next.detail));
  await opened.session.close(); // a no-op now
});

test("several messages on one connection", async () => {
  const s = smtpServer();
  const opened = await openSmtp(cfg(), s.connect);
  assert.ok(opened.ok);
  for (const to of ["a@example.com", "b@example.com", "c@example.com"]) {
    assert.deepEqual(await opened.session.send(msg({ to })), { ok: true });
  }
  await opened.session.close();
  assert.equal(s.state.connects.length, 1);
  assert.equal(s.state.messages.length, 3);
  assert.equal(s.state.log.filter((l) => l.line.startsWith("AUTH")).length, 1);
});

test("a connection dropped mid-session is temporary and ends the session", async () => {
  for (const stage of ["greeting", "EHLO", "RCPT", "DOT"] as Stage[]) {
    const s = smtpServer({ on: (st) => (st === stage ? "drop" : undefined) });
    const opened = await openSmtp(cfg(), s.connect);
    const r = opened.ok ? await opened.session.send(msg()) : opened;
    assert.ok(!r.ok && r.temporary && /connection lost at/.test(r.detail), `${stage}: ${!r.ok && r.detail}`);
    if (opened.ok) {
      const again = await opened.session.send(msg());
      assert.ok(!again.ok && /session is closed/.test(again.detail));
    }
  }
});

test("a server that stops answering times out as temporary", async () => {
  const { sent } = await sendOne({ on: (st) => (st === "DATA" ? "hang" : undefined) }, { timeoutMs: 30 });
  assert.ok(sent && !sent.ok && sent.temporary && sent.detail === "smtp: no reply at DATA within 30 ms");
  const { opened } = await sendOne({ on: (st) => (st === "greeting" ? "hang" : undefined) }, { timeoutMs: 30 });
  assert.ok(!opened.ok && opened.temporary && /no reply at greeting/.test(opened.detail));
});

test("a silent QUIT is not an error", async () => {
  const s = smtpServer({ on: (st) => (st === "QUIT" ? "hang" : undefined), closeThrows: true });
  const opened = await openSmtp(cfg({ timeoutMs: 30 }), s.connect);
  assert.ok(opened.ok);
  assert.deepEqual(await opened.session.send(msg()), { ok: true });
  await opened.session.close();
  assert.equal(s.state.closes, 1);
});

test("a garbled reply is temporary", async () => {
  const { opened } = await sendOne({ on: (st) => (st === "EHLO" ? "hello there" : undefined) });
  assert.ok(!opened.ok && opened.temporary && /malformed reply "hello there"/.test(opened.detail));
});

test("a connect that throws is temporary", async () => {
  const r = await openSmtp(cfg(), () => {
    throw new Error("no route to host");
  });
  assert.deepEqual(r, { ok: false, temporary: true, detail: "smtp: Error: no route to host" });
});

test("a write that fails is a lost connection", async () => {
  const s = smtpServer();
  const opened = await openSmtp(cfg(), (a, o) => {
    const sock = s.connect(a, o);
    return { ...sock, startTls: () => ({ ...sock.startTls(), writable: new WritableStream({ write: () => Promise.reject("EPIPE") }) }) };
  });
  assert.ok(!opened.ok && opened.temporary && /connection lost at EHLO: EPIPE/.test(opened.detail));
});
