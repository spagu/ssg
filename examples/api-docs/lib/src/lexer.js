/**
 * The kinds of token a {@link Lexer} produces.
 * @readonly
 * @enum {string}
 */
export const TokenKind = Object.freeze({ Word: "word", Space: "space", Punct: "punct" });

/**
 * A token: one run of characters of the same kind.
 * @typedef {Object} Token
 * @property {string} kind - one of {@link TokenKind}
 * @property {string} text - the characters
 * @property {number} offset - where the token starts in the source
 */

/**
 * Splits text into words, spaces and punctuation, one token at a time.
 *
 * @example
 * const lex = new Lexer("Hello, world");
 * lex.next(); // { kind: "word", text: "Hello", offset: 0 }
 */
export class Lexer {
  /**
   * @param {string} source - the text to read
   */
  constructor(source) {
    /** The text being read. @type {string} */
    this.source = source;
    this.offset = 0;
  }

  /**
   * Reads the next token.
   * @returns {Token | null} the token, or null at the end of the text
   */
  next() {
    if (this.offset >= this.source.length) return null;
    const start = this.offset;
    const kind = kindOf(this.source[start]);
    while (this.offset < this.source.length && kindOf(this.source[this.offset]) === kind) this.offset++;
    return { kind, text: this.source.slice(start, this.offset), offset: start };
  }

  /**
   * Reads every remaining token.
   * @returns {Token[]} the tokens, in order
   */
  all() {
    const out = [];
    for (let t = this.next(); t; t = this.next()) out.push(t);
    return out;
  }
}

/** @param {string} ch @returns {string} @internal */
function kindOf(ch) {
  if (/\s/.test(ch)) return TokenKind.Space;
  if (/[\p{L}\p{N}]/u.test(ch)) return TokenKind.Word;
  return TokenKind.Punct;
}
