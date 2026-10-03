<?php
namespace Acme\Textkit\Token;

/**
 * Anything that yields tokens.
 *
 * @beta
 */
interface Tokenizer extends \Countable, \Stringable
{
    /** Separator between tokens. */
    const SEPARATOR = ' ';

    /**
     * Reads the next kind.
     */
    function next(): ?Kind;
}

/**
 * Remembers where a token was found.
 */
trait Positioned
{
    public int $line = 1;

    public function position(): int
    {
        return $this->line;
    }
}

/**
 * Not documented: hidden from the API.
 *
 * @ignore
 */
class Hidden
{
}
