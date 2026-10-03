/**
 * The public entry point of lexkit.
 *
 * @module lexkit
 */

export { Lexer, TokenStream, tokenize as tokenise, MAX_DEPTH, first } from './lexer.js';
export { default } from './lexer.js';
export * from './util.js';
export * from './cycle.js';
export * as legacy from './legacy.cjs';
export { leftPad } from 'left-pad';
export { secret, debugDump } from './lexer.js';
import './broken.js';
