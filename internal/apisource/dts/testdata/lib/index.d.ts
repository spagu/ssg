/**
 * Tokens and the lexer that reads them.
 * @packageDocumentation
 */
import { Lexer } from "./lexer";

export { Lexer, Shape, Token, TokenKind, type Visitor, lexers, default as MainLexer } from "./lexer";
export * from "./util";
export * as lexing from "./lexer.js";
export { broken } from "./broken";
export { missing } from "./nowhere";

/**
 * Parses text with the default lexer.
 * @param src the text
 */
export default function parse(src: string): Lexer;

export declare function oops(: string): void;

declare module "lib/plugins" {
    /** A plugin. */
    export interface Plugin {
        name: string;
        apply(lexer: Lexer): void;
    }
}
