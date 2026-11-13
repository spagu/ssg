// The message every mail transport sends, its safety checks, and its MIME
// rendering for SMTP. A leading underscore keeps this file out of the Pages
// route table — it is imported, never served.
//
// Portable: _mime.ts, _smtp.ts and _transport.ts import nothing else from the
// worker, so another template can carry byte-identical copies of the three.

export interface MailMessage {
  to: string;
  subject: string;
  text: string; // always sent; the HTML part is an alternative to it
  html?: string;
  headers?: Record<string, string>; // e.g. List-Unsubscribe, List-Unsubscribe-Post, List-Id
  from?: string; // defaults to NEWSLETTER_MAIL_FROM
  fromName?: string; // defaults to NEWSLETTER_MAIL_FROM_NAME
  // The same key for every attempt at the same message (e.g. "campaign:subscriber").
  // Resend receives it as the Idempotency-Key header and sends a message at most
  // once per key within 24 hours; SMTP gets a Message-ID derived from it, which
  // only helps receivers that de-duplicate. Absent: the request is unchanged.
  idempotencyKey?: string;
}

// A message whose sender has been resolved — what a transport actually sends.
export interface Outgoing extends MailMessage {
  from: string;
  fromName: string;
}

// temporary = worth retrying later (4xx, network, timeout); permanent = the
// same message will fail the same way again (5xx, bad config, bad input).
export type MailFailure = { ok: false; temporary: boolean; detail: string };
// duplicate: the provider refused this request because a message under the
// same idempotencyKey was accepted earlier — nothing new left, but the message
// did. Callers that only read `ok` treat it as sent, which is what it is.
export type MailResult = { ok: true; duplicate?: true } | MailFailure;

export const permanent = (detail: string): MailFailure => ({ ok: false, temporary: false, detail });
export const temporary = (detail: string): MailFailure => ({ ok: false, temporary: true, detail });

// Headers the renderer writes itself. Letting a caller set them too would give
// a message two From: or two Content-Type: lines.
const OWN_HEADERS = new Set([
  "from", "to", "cc", "bcc", "subject", "date", "message-id", "mime-version", "content-type", "content-transfer-encoding",
]);

// checkMessage refuses header injection rather than repairing it: a CR or LF
// in an address, the subject, the display name or a header value is either a
// bug or an attack, and stripping it silently would send something nobody
// wrote. Returns the problem, or null when the message is safe to send.
export function checkMessage(m: Outgoing): string | null {
  const fields: [string, string][] = [["from", m.from], ["to", m.to], ["display name", m.fromName], ["subject", m.subject]];
  for (const [what, value] of fields) {
    if (/[\r\n]/.test(value)) return `CR or LF in ${what}`;
  }
  // Visible ASCII, 1–256 characters: what Resend accepts, and safe in a header.
  if (m.idempotencyKey !== undefined && !/^[!-~]{1,256}$/.test(m.idempotencyKey)) return "invalid idempotency key";
  for (const [name, value] of Object.entries(m.headers ?? {})) {
    // RFC 5322 field name: printable ASCII except ":"; short enough that the
    // header line stays far below the 998-octet limit.
    if (!/^[!-9;-~]{1,76}$/.test(name)) return `invalid header name ${JSON.stringify(name)}`;
    if (OWN_HEADERS.has(name.toLowerCase())) return `header ${name} is set by the mailer`;
    if (/[\r\n]/.test(value)) return `CR or LF in header ${name}`;
  }
  return null;
}

// isAddress accepts a bare ASCII addr-spec, as MAIL FROM and RCPT TO need it.
// Non-ASCII addresses would need SMTPUTF8, which this client does not speak.
export function isAddress(addr: string): boolean {
  return /^[!-~]+$/.test(addr) && /^[^@\s<>()[\]",;:\\]+@[^@\s<>()[\]",;:\\]+$/.test(addr);
}

const encoder = new TextEncoder();

function base64(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

// encodedWords writes RFC 2047 "B" words of at most 45 UTF-8 bytes (60 base64
// characters, 72 with the wrapper), split between code points so no character
// is cut in half, and folded one word per line.
function encodedWords(s: string): string {
  const words: string[] = [];
  let word = "";
  let size = 0;
  for (const ch of s) {
    const n = encoder.encode(ch).length;
    if (size + n > 45) {
      words.push(word);
      word = "";
      size = 0;
    }
    word += ch;
    size += n;
  }
  words.push(word);
  return words.map((w) => `=?UTF-8?B?${base64(encoder.encode(w))}?=`).join("\r\n ");
}

// headerText leaves printable ASCII alone and encodes the rest. A very long
// ASCII value is encoded too, because encoded words fold and plain text here
// does not.
export function headerText(s: string): string {
  return /^[\x20-\x7e]*$/.test(s) && s.length <= 900 ? s : encodedWords(s);
}

function mailbox(name: string, addr: string): string {
  if (!name) return addr;
  if (/^[\x20-\x7e]*$/.test(name) && name.length <= 200) return `"${name.replace(/[\\"]/g, "\\$&")}" <${addr}>`;
  return `${encodedWords(name)} <${addr}>`;
}

// quotedPrintable keeps a body readable on the wire while bounding every line
// to 76 octets, whatever the text holds: one long line or none at all.
export function quotedPrintable(text: string): string {
  return text.replace(/\r\n?/g, "\n").split("\n").map(qpLine).join("\r\n");
}

function qpLine(line: string): string {
  const bytes = encoder.encode(line);
  let out = "";
  let cur = "";
  for (let i = 0; i < bytes.length; i++) {
    const b = bytes[i];
    const literal = (b >= 33 && b <= 126 && b !== 61) || ((b === 32 || b === 9) && i < bytes.length - 1);
    const token = literal ? String.fromCharCode(b) : `=${b.toString(16).toUpperCase().padStart(2, "0")}`;
    if (cur.length + token.length > 75) {
      out += `${cur}=\r\n`; // soft line break
      cur = "";
    }
    cur += token;
  }
  return out + cur;
}

// messageId is the Message-ID's left part: the idempotency key when there is
// one — every character outside [A-Za-z0-9_-] written as =XX, so the result is
// a valid dot-atom and the same key always gives the same id — else `id`.
export function messageId(m: Outgoing, id: string): string {
  if (m.idempotencyKey === undefined) return id;
  return m.idempotencyKey.replace(/[^A-Za-z0-9_-]/g, (c) => `=${c.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0")}`);
}

const part = (type: string, body: string): string =>
  `Content-Type: ${type}; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n${quotedPrintable(body)}\r\n`;

// buildMime renders a checked message: CRLF line endings, every line at most
// 998 octets, ending in CRLF. Dot-stuffing is the SMTP layer's job. `id`
// names the boundary, and the Message-ID unless the message has a key.
export function buildMime(m: Outgoing, now = new Date(), id: string = crypto.randomUUID()): string {
  const head = [
    `From: ${mailbox(m.fromName, m.from)}`,
    `To: ${m.to}`,
    `Subject: ${headerText(m.subject)}`,
    `Date: ${now.toUTCString().replace("GMT", "+0000")}`,
    `Message-ID: <${messageId(m, id)}@${m.from.slice(m.from.lastIndexOf("@") + 1)}>`,
    "MIME-Version: 1.0",
    ...Object.entries(m.headers ?? {}).map(([name, value]) => `${name}: ${headerText(value)}`),
  ];
  if (!m.html) return `${head.join("\r\n")}\r\n${part("text/plain", m.text)}`;
  // "=_" never occurs in quoted-printable output, so no body line can be
  // mistaken for the boundary.
  const boundary = `=_${id}`;
  head.push(`Content-Type: multipart/alternative; boundary="${boundary}"`);
  return (
    `${head.join("\r\n")}\r\n\r\n` +
    `--${boundary}\r\n${part("text/plain", m.text)}` +
    `--${boundary}\r\n${part("text/html", m.html)}` +
    `--${boundary}--\r\n`
  );
}
