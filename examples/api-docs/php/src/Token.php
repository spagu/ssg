<?php

namespace Textkit;

/**
 * One piece of the input.
 */
final class Token
{
    /**
     * @param Kind   $kind  What the token is.
     * @param string $text  The token as written.
     * @param int    $start Byte offset of the first character.
     */
    public function __construct(
        public readonly Kind $kind,
        public readonly string $text,
        public readonly int $start,
    ) {
    }
}

/**
 * What a {@link Token} is.
 */
enum Kind: string
{
    case Word = 'word';
    case Number = 'number';
    case Punct = 'punct';
    case Space = 'space';
}
