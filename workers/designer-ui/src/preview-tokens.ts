// The look the preview applies (GO-188): the theme's colour and font tokens
// as the panel holds them before publishing, sent as query parameters
// "--name=value", turned into one stylesheet. Only custom properties with a
// plain value pass: the value lands inside a <style> element of a page served
// from the panel's origin, so it may not close the element, the declaration
// or the rule, and may not load anything from another host: no url(), no
// @import, and no "//" — image-set() and similar take a quoted string as an
// address, and without ":" (no scheme) and "//" (no host) such a string can
// only name a file of this site (SR-22).

import { PreviewError } from "./preview-fetch";

export const MAX_TOKENS = 200;
const TOKEN_NAME = /^--[A-Za-z0-9_-]{1,100}$/;
// A hex colour, a colour function, a font stack: letters, digits, spaces,
// # , . % ( ) + - / _ and quotes. No ; { } < > \ @ : — nothing that ends
// the declaration or starts another construct.
const TOKEN_VALUE = /^[A-Za-z0-9 #,.%()+\-/_'"]{1,200}$/;

// tokensFrom: the overrides in the query, in order; bad_token (400) for a
// parameter that is not one, or too many.
export function tokensFrom(params: URLSearchParams): [string, string][] {
  const out: [string, string][] = [];
  for (const [name, value] of params) {
    if (!name.startsWith("--")) continue;
    const v = value.trim();
    if (!TOKEN_NAME.test(name) || !TOKEN_VALUE.test(v) || /url\(|expression\(|\/\//i.test(v)) throw new PreviewError("bad_token", 400);
    out.push([name, v]);
  }
  if (out.length > MAX_TOKENS) throw new PreviewError("bad_token", 400);
  return out;
}

// tokensCSS: one rule on :root, specific enough (0,4,0) to win over a
// theme's :root[data-theme="dark"] wherever the theme's own rules sit. The
// colour scheme is set with it, so form controls and scroll bars follow.
export function tokensCSS(tokens: [string, string][], scheme: "light" | "dark" | null): string {
  const decls = tokens.map(([n, v]) => `${n}:${v}`);
  if (scheme) decls.push(`color-scheme:${scheme}`);
  return decls.length ? `:root:root:root:root{${decls.join(";")}}` : "";
}
