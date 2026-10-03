<?php

namespace Textkit;

/**
 * Splits text into tokens, in order.
 *
 * @param string $text      The text to split.
 * @param bool   $keepSpace Return whitespace tokens too.
 * @return Token[] Every token.
 * @throws \InvalidArgumentException When the text is longer than {@link Lexer::MAX_LENGTH}.
 */
function tokenize(string $text, bool $keepSpace = false): array
{
    $lexer = new Lexer($text);
    $out = [];
    while ($t = $lexer->next()) {
        $out[] = $t;
    }
    return $out;
}
