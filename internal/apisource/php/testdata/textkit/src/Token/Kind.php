<?php
namespace Acme\Textkit\Token;

/**
 * What a token is.
 */
enum Kind: string implements \JsonSerializable
{
    /** A word. */
    case Word = 'W';
    case Space = ' ';

    const DEFAULT = self::Word;

    /**
     * Tells whether the kind carries text.
     */
    public function hasText(): bool
    {
        return match ($this) { self::Word => true, default => false };
    }

    public function jsonSerialize(): mixed
    {
        return $this->value;
    }
}
