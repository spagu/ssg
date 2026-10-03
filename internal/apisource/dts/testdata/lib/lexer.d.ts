import type { Options } from "./util";

/** The kinds of token. */
export declare enum TokenKind {
    /** A name. */
    Ident = 1,
    Number,
    Str = "string"
}

/**
 * One token.
 * @template V the value type
 */
export interface Token<V = string> {
    readonly kind: TokenKind;
    value?: V;
    readonly [extra: string]: unknown;
}

/** Visits tokens. */
export interface Visitor<T extends Token = Token> {
    /** Called for each token. */
    (token: T, index: number): boolean;
    new (opts: Options): Visitor<T>;
    done?(): void
    label: string
}

declare class Base {
    close(): void;
}

/**
 * Splits text into tokens.
 * @beta
 */
export declare class Lexer<T extends Token = Token> extends Base implements Iterable<T>, Disposable {
    #private;
    /**
     * Creates a lexer.
     * @param source the text
     */
    constructor(source: string);
    /** Creates a lexer with options. */
    constructor(source: string, options: Options);
    /** Number of tokens read. */
    readonly count: number;
    static readonly version: string;
    protected buffer: string[];
    private secret;
    /** @internal */
    debug: boolean;
    get position(): number;
    set position(value: number);
    get done(): boolean;
    /**
     * Reads the next token.
     * @returns the token, or undefined at the end
     */
    next(): T | undefined;
    peek<K extends keyof T>(key: K, ...rest: K[]): T[K];
    [Symbol.iterator](): Iterator<T>;
    static create(source: string): Lexer;
}

/** A shape with an area. */
export declare abstract class Shape {
    abstract area(): number;
}

/** Lexers by name. */
export declare namespace lexers {
    const all: Lexer[];
    function make(kind: TokenKind): Lexer;
    namespace deep {
        type Id = string;
    }
}

/** @hidden */
export declare function hiddenThing(): void;

export default Lexer;
