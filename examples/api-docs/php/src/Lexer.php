<?php

declare(strict_types=1);

namespace Textkit;

/**
 * Reads text one token at a time.
 *
 * @example
 * ```php
 * $lexer = new Lexer("Hello, world");
 * while ($token = $lexer->next()) {
 *     echo $token->text, "\n";
 * }
 * ```
 */
final class Lexer
{
    /** The longest input the lexer accepts, in bytes. */
    public const MAX_LENGTH = 100000;

    private int $pos = 0;

    /**
     * @param string $source The text to read.
     */
    public function __construct(private readonly string $source)
    {
    }

    /**
     * Returns the next token, or null at the end of the input.
     *
     * @return Token|null The token, or null when nothing is left.
     */
    public function next(): ?Token
    {
        if ($this->pos >= strlen($this->source)) {
            return null;
        }
        $text = $this->source[$this->pos];
        return new Token(Kind::Word, $text, $this->pos++);
    }
}
