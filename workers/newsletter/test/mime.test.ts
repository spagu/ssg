// Message checks and MIME rendering. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { buildMime, checkMessage, headerText, isAddress, quotedPrintable } from "../functions/api/newsletter/_mime.ts";
import type { Outgoing } from "../functions/api/newsletter/_mime.ts";

const msg = (over: Partial<Outgoing> = {}): Outgoing => ({
  from: "news@example.com",
  fromName: "",
  to: "reader@example.com",
  subject: "Hello",
  text: "Body",
  ...over,
});
const when = new Date(Date.UTC(2026, 10, 13, 9, 5, 7));

// decodeWords undoes RFC 2047 "B" words, joining folded ones.
const decodeWords = (s: string): string =>
  new TextDecoder().decode(
    Uint8Array.from(
      [...s.matchAll(/=\?UTF-8\?B\?([^?]*)\?=/g)].flatMap((m) => [...atob(m[1])].map((c) => c.charCodeAt(0))),
    ),
  );

test("plain text: headers, CRLF, quoted-printable", () => {
  const out = buildMime(msg({ fromName: "Example News", headers: { "List-Unsubscribe-Post": "List-Unsubscribe=One-Click" } }), when, "id-1");
  assert.equal(
    out,
    'From: "Example News" <news@example.com>\r\n' +
      "To: reader@example.com\r\n" +
      "Subject: Hello\r\n" +
      "Date: Fri, 13 Nov 2026 09:05:07 +0000\r\n" +
      "Message-ID: <id-1@example.com>\r\n" +
      "MIME-Version: 1.0\r\n" +
      "List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n" +
      "Content-Type: text/plain; charset=utf-8\r\n" +
      "Content-Transfer-Encoding: quoted-printable\r\n" +
      "\r\n" +
      "Body\r\n",
  );
  assert.ok(!/[^\r]\n/.test(out), "every line ends in CRLF");
});

test("with HTML: multipart/alternative, text first", () => {
  const out = buildMime(msg({ html: "<p>Body</p>" }), when, "id-2");
  assert.match(out, /^From: news@example\.com\r\n/);
  assert.match(out, /Content-Type: multipart\/alternative; boundary="=_id-2"\r\n\r\n--=_id-2\r\nContent-Type: text\/plain/);
  assert.match(out, /\r\n--=_id-2\r\nContent-Type: text\/html; charset=utf-8\r\n/);
  assert.ok(out.endsWith("\r\n--=_id-2--\r\n"));
  assert.ok(out.indexOf("text/plain") < out.indexOf("text/html"));
});

test("default Date and Message-ID", () => {
  const out = buildMime(msg());
  assert.match(out, /\r\nMessage-ID: <[0-9a-f-]{36}@example\.com>\r\n/);
  assert.match(out, /\r\nDate: \w{3}, \d{2} \w{3} \d{4} \d{2}:\d{2}:\d{2} \+0000\r\n/);
});

test("non-ASCII subject and display name become encoded words", () => {
  const subject = "Zażółć gęślą jaźń — newsletter 🎉 ".repeat(4);
  const out = buildMime(msg({ subject, fromName: "Redakcja Łódź" }), when, "x");
  const subj = /\r\nSubject: ((?:.*\r\n )*.*)\r\n/.exec(out)![1];
  assert.equal(decodeWords(subj), subject);
  for (const line of subj.split("\r\n")) assert.ok(line.length <= 78, line);
  assert.match(out, /^From: =\?UTF-8\?B\?[^?]+\?= <news@example\.com>\r\n/);
  assert.equal(decodeWords(out.split("\r\n")[0]), "Redakcja Łódź");
});

test("an ASCII display name is quoted and escaped; a huge one encoded", () => {
  assert.match(buildMime(msg({ fromName: 'The "Best" \\ News' })), /^From: "The \\"Best\\" \\\\ News" <news@example\.com>/);
  assert.match(buildMime(msg({ fromName: "n".repeat(300) })), /^From: =\?UTF-8\?B\?/);
});

test("headerText: short ASCII as is, long or non-ASCII encoded", () => {
  assert.equal(headerText("<https://example.com/u?t=1>"), "<https://example.com/u?t=1>");
  assert.equal(decodeWords(headerText("ü")), "ü");
  const long = "a".repeat(1000);
  assert.equal(decodeWords(headerText(long)), long);
});

test("quotedPrintable: =, trailing space, UTF-8, soft breaks", () => {
  assert.equal(quotedPrintable("a=b"), "a=3Db");
  assert.equal(quotedPrintable("end \nx\t"), "end=20\r\nx=09");
  assert.equal(quotedPrintable("ż"), "=C5=BC");
  assert.equal(quotedPrintable("a\r\nb\rc"), "a\r\nb\r\nc");
  const long = quotedPrintable("y".repeat(200));
  assert.ok(long.split("\r\n").every((l) => l.length <= 76));
  assert.equal(long.replaceAll("=\r\n", ""), "y".repeat(200));
  // An =XX triplet is never split by a soft break.
  const enc = quotedPrintable("ż".repeat(60));
  assert.ok(enc.split("\r\n").every((l) => /^(=[0-9A-F]{2})*=?$/.test(l)));
});

test("checkMessage refuses CR/LF anywhere and headers it owns", () => {
  assert.equal(checkMessage(msg()), null);
  assert.equal(checkMessage(msg({ headers: { "List-Id": "News <news.example.com>" } })), null);
  assert.equal(checkMessage(msg({ to: "a@example.com\r\nBcc: b@example.com" })), "CR or LF in to");
  assert.equal(checkMessage(msg({ from: "a@example.com\n" })), "CR or LF in from");
  assert.equal(checkMessage(msg({ fromName: "x\ry" })), "CR or LF in display name");
  assert.equal(checkMessage(msg({ subject: "x\ny" })), "CR or LF in subject");
  assert.equal(checkMessage(msg({ headers: { "X-A": "1\r\nBcc: b@example.com" } })), "CR or LF in header X-A");
  assert.equal(checkMessage(msg({ headers: { "X-A\r\nBcc": "b" } })), 'invalid header name "X-A\\r\\nBcc"');
  assert.equal(checkMessage(msg({ headers: { "X A": "b" } })), 'invalid header name "X A"');
  assert.equal(checkMessage(msg({ headers: { "X:A": "b" } })), 'invalid header name "X:A"');
  assert.equal(checkMessage(msg({ headers: { "": "b" } })), 'invalid header name ""');
  assert.equal(checkMessage(msg({ headers: { bcc: "b@example.com" } })), "header bcc is set by the mailer");
  assert.equal(checkMessage(msg({ headers: { "Content-Type": "text/html" } })), "header Content-Type is set by the mailer");
});

test("isAddress: plain ASCII addr-spec only", () => {
  assert.ok(isAddress("reader+tag@example.com"));
  for (const bad of ["reader", "a@b@example.com", "a b@example.com", "<a@example.com>", "zażółć@example.com", "a@example.com\r\n", "a@"]) {
    assert.ok(!isAddress(bad), bad);
  }
});
