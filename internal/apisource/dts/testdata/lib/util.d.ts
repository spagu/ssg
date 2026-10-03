import { Lexer, Token } from "./lexer";

/** Options for the lexer. */
export interface Options {
    /** Treat errors as fatal. */
    strict?: boolean;
}

/**
 * Tokenizes text.
 * @param text the source
 * @returns the tokens
 */
export declare function tokenize(text: string): Token[];
/**
 * Tokenizes with a lexer.
 * @param lexer the lexer to use
 */
export declare function tokenize(text: string, lexer: Lexer): Token[];

/**
 * Maps values.
 * @template T the input
 */
export declare function map<T, const U extends object = {}>(items: readonly T[], fn: (item: T) => U): U[];

export type Pair<A, B = A> = [A, B];
export type Mode =
    | "fast"
    | "safe";

export declare const VERSION: "1.2.3";
export declare let level: number, verbose: boolean;

/** @alpha */
export declare function experimental(this: Window, x?: number): asserts x is number;
