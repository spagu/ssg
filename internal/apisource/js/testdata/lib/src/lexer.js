/**
 * A token produced by the lexer.
 *
 * @typedef {Object} Token
 * @property {string} type - The kind of token.
 * @property {string} value - The text it covers.
 * @property {number} [line=1] - Where it starts.
 */

/**
 * Called once for every token.
 *
 * @callback TokenHandler
 * @param {Token} token - The token just read.
 * @returns {boolean} False to stop.
 */

/** @typedef {'word' | 'space' | 'number'} TokenType */

/**
 * Splits source text into tokens.
 *
 * @example
 * const lx = new Lexer('a b');
 */
export class Lexer {
  /** @type {number} Where the lexer is. */
  position = 0;

  /** The lexer's version. */
  static VERSION = '1.0';

  #buffer = [];

  /**
   * Creates a lexer.
   *
   * @param {string} source - The text to split.
   * @param {Object} [options] - Settings.
   */
  constructor(source, options = {}) {
    this.source = source;
    this.options = options;
  }

  /**
   * Whether the end was reached.
   *
   * @returns {boolean}
   */
  get done() {
    return this.position >= this.source.length;
  }

  set done(value) {
    this.position = value ? this.source.length : 0;
  }

  /**
   * Yields the tokens one by one.
   *
   * @returns {AsyncGenerator<Token>}
   */
  async *tokens() {
    yield* this.#buffer;
  }

  /** @private */
  reset() {
    this.position = 0;
  }

  /** @internal */
  debug() {
    return `${this.position}/${this.source.length}`;
  }

  /**
   * Builds a lexer.
   *
   * @param {string} source
   * @returns {Lexer}
   */
  static from(source) {
    return new Lexer(source);
  }
}

/**
 * A lexer that keeps every token.
 *
 * @beta
 */
export class TokenStream extends Lexer {
  /** @type {Token[]} */
  tokens = [];
}

/**
 * Splits a whole text at once.
 *
 * @param {string} source - The text.
 * @param {TokenHandler} [handler] - Called for each token.
 * @returns {Promise<Token[]>} Every token.
 */
export async function tokenize(source, handler) {
  const re = /[{(]/g;
  return [source, handler, re];
}

/**
 * The deepest nesting the lexer follows.
 *
 * @beta
 */
export const MAX_DEPTH = 64;

/**
 * Returns the first item.
 *
 * @template T
 * @param {T[]} items - The list.
 * @returns {T | undefined} Its first item.
 */
export const first = (items) => items[0];

/** @hidden */
export function secret() {}

/**
 * Dumps the state.
 *
 * @internal
 */
export function debugDump() {}

/**
 * Creates a lexer.
 *
 * @param {string} source - The text.
 * @returns {Lexer} A new lexer.
 */
export default function createLexer(source) {
  return new Lexer(source);
}
