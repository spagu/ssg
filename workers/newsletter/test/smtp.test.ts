// The SMTP client against the in-memory server: TLS, login, the happy path
// and what goes on the wire. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { openSmtp, dotStuff } from "../functions/api/newsletter/_smtp.ts";
import type { SmtpConfig } from "../functions/api/newsletter/_smtp.ts";
import type { Outgoing } from "../functions/api/newsletter/_mime.ts";
import { smtpServer } from "./fakes.ts";
import { cfg, msg, sendOne } from "./smtpcase.ts";

test("587: STARTTLS, EHLO again, AUTH PLAIN, one message, QUIT", async () => {
  const { s, sent } = await sendOne();
  assert.deepEqual(sent, { ok: true });
  assert.deepEqual(s.state.connects[0], {
    address: { hostname: "smtp.example.com", port: 587 },
    options: { secureTransport: "starttls", allowHalfOpen: false },
  });
  assert.equal(s.state.startTls, 1);
  const lines = s.state.log.map((l) => l.line.split(" ")[0]);
  assert.deepEqual(lines, ["EHLO", "STARTTLS", "EHLO", "AUTH", "MAIL", "RCPT", "DATA", "QUIT"]);
  assert.equal(s.state.log[0].line, "EHLO example.com");
  assert.equal(s.state.log[4].line, "MAIL FROM:<news@example.com>");
  assert.equal(s.state.log[5].line, "RCPT TO:<reader@example.com>");
  // Nothing after STARTTLS travelled in clear text, credentials least of all.
  assert.ok(s.state.log.slice(2).every((l) => l.tls));
  assert.match(s.state.messages[0], /^From: "Example News" <news@example\.com>\r\n/);
  assert.ok(s.state.closes >= 1);
});

test("465: TLS from the first byte, no STARTTLS", async () => {
  const { s, sent } = await sendOne({}, { port: 465 });
  assert.deepEqual(sent, { ok: true });
  assert.equal(s.state.connects[0].options.secureTransport, "on");
  assert.equal(s.state.startTls, 0);
  assert.ok(s.state.log.every((l) => l.tls));
  assert.ok(!s.state.log.some((l) => l.line === "STARTTLS"));
});

test("another port is treated as STARTTLS", async () => {
  const { s, sent } = await sendOne({}, { port: 2525 });
  assert.deepEqual(sent, { ok: true });
  assert.equal(s.state.connects[0].options.secureTransport, "starttls");
});

test("port 25 is refused before any connection", async () => {
  const s = smtpServer();
  const r = await openSmtp(cfg({ port: 25 }), s.connect);
  assert.equal(r.ok, false);
  assert.ok(!r.ok && !r.temporary && /port 25 is blocked by Cloudflare/.test(r.detail));
  assert.equal(s.state.connects.length, 0);
});

test("configuration no server could fix is permanent", async () => {
  for (const [over, re] of [
    [{ port: Number.NaN }, /invalid port/],
    [{ port: 70000 }, /invalid port/],
    [{ helo: "bad name" }, /invalid EHLO/],
    [{ helo: "x\r\nRCPT TO:<a@example.com>" }, /invalid EHLO/],
    [{ password: undefined }, /password is not/],
  ] as [Partial<SmtpConfig>, RegExp][]) {
    const r = await openSmtp(cfg(over), smtpServer().connect);
    assert.ok(!r.ok && !r.temporary && re.test(r.detail), JSON.stringify(over));
  }
});

test("without STARTTLS on 587 nothing is sent in clear text", async () => {
  const { s, opened } = await sendOne({ starttls: false });
  assert.ok(!opened.ok && !opened.temporary && /does not offer STARTTLS/.test(opened.detail));
  assert.deepEqual(s.state.log.map((l) => l.line), ["EHLO example.com"]);
  assert.equal(s.state.closes, 1);
});

test("data pushed before the TLS handshake is refused", async () => {
  const { s, opened } = await sendOne({ afterStartTls: "250 injected" });
  assert.ok(!opened.ok && !opened.temporary && /unexpected data after the STARTTLS/.test(opened.detail));
  assert.equal(s.state.startTls, 0);
});

test("AUTH LOGIN when PLAIN is not offered", async () => {
  const { s, sent } = await sendOne({ auth: "LOGIN" });
  assert.deepEqual(sent, { ok: true });
  const auth = s.state.log.filter((l) => !/^(EHLO|STARTTLS|MAIL|RCPT|DATA|QUIT)/.test(l.line));
  assert.deepEqual(auth.map((l) => l.line), ["AUTH LOGIN", btoa("mailer@example.com"), btoa("s3cret")]);
  assert.ok(auth.every((l) => l.tls));
});

test("the old AUTH=PLAIN spelling is understood", async () => {
  const { sent } = await sendOne({ auth: "=PLAIN" });
  assert.deepEqual(sent, { ok: true });
});

test("AUTH rejected is permanent, for PLAIN and LOGIN", async () => {
  for (const auth of ["PLAIN", "LOGIN"]) {
    const { opened, s } = await sendOne({ auth }, { password: "wrong" });
    assert.ok(!opened.ok && !opened.temporary && /^smtp 535 at AUTH/.test(opened.detail), auth);
    assert.ok(!s.state.log.some((l) => l.line.startsWith("MAIL")));
  }
});

test("no usable AUTH mechanism is permanent", async () => {
  const { opened } = await sendOne({ auth: "CRAM-MD5" });
  assert.ok(!opened.ok && !opened.temporary && /neither AUTH PLAIN nor AUTH LOGIN/.test(opened.detail));
});

test("a user set but no AUTH advertised is permanent", async () => {
  const { opened } = await sendOne({ auth: "" });
  assert.ok(!opened.ok && !opened.temporary && /neither AUTH PLAIN nor AUTH LOGIN/.test(opened.detail));
});

test("without a user there is no AUTH", async () => {
  const { s, sent } = await sendOne({ auth: "" }, { user: undefined, password: undefined });
  assert.deepEqual(sent, { ok: true });
  assert.ok(!s.state.log.some((l) => l.line.startsWith("AUTH")));
});

test("header injection is refused before anything is sent", async () => {
  const s = smtpServer();
  const opened = await openSmtp(cfg(), s.connect);
  assert.ok(opened.ok);
  for (const bad of [
    { to: "reader@example.com\r\nBcc: victim@example.com" },
    { from: "news@example.com\nX: y" },
    { fromName: "Example\r\nBcc: victim@example.com" },
    { subject: "Hi\r\nBcc: victim@example.com" },
    { headers: { "List-Id": "x\r\nBcc: victim@example.com" } },
  ] as Partial<Outgoing>[]) {
    const r = await opened.session.send(msg(bad));
    assert.ok(!r.ok && !r.temporary && /CR or LF/.test(r.detail), JSON.stringify(bad));
  }
  const notAscii = await opened.session.send(msg({ to: "zażółć@example.com" }));
  assert.ok(!notAscii.ok && !notAscii.temporary && /plain ASCII address/.test(notAscii.detail));
  await opened.session.close();
  assert.ok(!s.state.log.some((l) => l.line.startsWith("MAIL")));
});

test("dot-stuffing and long lines on the wire", async () => {
  const long = "x".repeat(5000);
  const { s, sent } = await sendOne({}, {}, { text: `.hidden\n..two\nok.\n${long}`, html: `<p>${long}</p>\n.\n` });
  assert.deepEqual(sent, { ok: true });
  const wire = s.state.messages[0];
  assert.match(wire, /\r\n\.\.hidden\r\n/);
  assert.match(wire, /\r\n\.\.\.two\r\n/);
  assert.match(wire, /\r\n\.\.\r\n/); // a lone "." in the HTML would have ended DATA
  assert.ok(wire.split("\r\n").every((l) => l.length <= 998));
  assert.equal(dotStuff(".a\r\nb\r\n.c"), "..a\r\nb\r\n..c");
});

test("a non-ASCII subject travels as encoded words", async () => {
  const { s, sent } = await sendOne({}, {}, { subject: "Zażółć gęślą jaźń" });
  assert.deepEqual(sent, { ok: true });
  assert.match(s.state.messages[0], /\r\nSubject: =\?UTF-8\?B\?[A-Za-z0-9+/=]+\?=\r\n/);
});
