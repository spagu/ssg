// Link rewriting for click counting (GO-129): pure functions the renderer
// calls, so the preview, the test send and every recipient's mail go through
// the same rewrite.
//
// A LinkRewriter is asked about every http(s) link of a message and answers
// the URL to put in its place, or null to leave it alone. What is never
// offered to it, whatever it would answer:
//   - mailto: and every other non-http(s) scheme (a mail address is not a page);
//   - the unsubscribe link, in any form: its token identifies the reader, and
//     mailbox providers POST to it (RFC 8058) — a redirect would break that.

import type { HtmlNode } from "./sanitize";

export type LinkRewriter = (url: string) => string | null;

const HTTP = /^https?:\/\//i;

// A bare URL in plain text, up to whitespace or a character that cannot end
// one in prose. Trailing sentence punctuation is given back (see rewriteText).
const TEXT_URL = /https?:\/\/[^\s<>"'()]+/gi;
const TRAILING = /[.,;:!?]+$/;

// guarded wraps a rewriter so it only ever sees http(s) links that are not
// an unsubscribe link of `unsubscribePrefix` (the endpoint without its token).
export function guarded(rewrite: LinkRewriter, unsubscribePrefix: string): LinkRewriter {
  return (url) => (HTTP.test(url) && !url.startsWith(unsubscribePrefix) ? rewrite(url) : null);
}

// rewriteTree returns a copy of the sanitized tree with every <a href> the
// rewriter answers for replaced. Images are not links and are left alone.
export function rewriteTree(nodes: HtmlNode[], rewrite: LinkRewriter): HtmlNode[] {
  return nodes.map((n) => {
    if (n.type === "text") return n;
    const attrs = n.tag === "a" ? n.attrs.map(([k, v]): [string, string] => [k, k === "href" ? (rewrite(v) ?? v) : v]) : n.attrs;
    return { ...n, attrs, children: rewriteTree(n.children, rewrite) };
  });
}

// rewriteText replaces the bare http(s) URLs of a plain-text body (the
// admin's text_override), keeping the punctuation that ends a sentence.
export function rewriteText(text: string, rewrite: LinkRewriter): string {
  return text.replace(TEXT_URL, (found) => {
    const tail = TRAILING.exec(found)?.[0] ?? "";
    const url = found.slice(0, found.length - tail.length);
    return (rewrite(url) ?? url) + tail;
  });
}
