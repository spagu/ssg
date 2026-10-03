import { Lexer, TokenKind } from "./lexer.js";

/**
 * Turns a title into a URL segment: lower case, words joined by hyphens.
 *
 * @param {string} title - the title to convert
 * @returns {string} the slug
 * @example
 * slugify("Hello, World!"); // "hello-world"
 */
export function slugify(title) {
  return new Lexer(title).all()
    .filter((t) => t.kind === TokenKind.Word)
    .map((t) => t.text.toLowerCase())
    .join("-");
}

/**
 * Shortens text to at most `max` characters, at a word boundary.
 *
 * @param {string} text - the text to shorten
 * @param {number} [max=80] - the longest result, ellipsis included
 * @returns {string} the text, shortened with "…" when it was longer
 * @see {@link slugify} for URL segments
 */
export function truncate(text, max = 80) {
  if (text.length <= max) return text;
  const cut = text.slice(0, max - 1);
  return cut.slice(0, cut.lastIndexOf(" ")) + "…";
}
