<?php
namespace Acme\Textkit;

use Acme\Textkit\Lexer\Lexer;

/** Version of the library. */
const VERSION = '2.1.0';

/**
 * Splits text into words.
 *
 * @param string $text The text.
 * @param bool $keepSpace Whether to keep the spaces.
 * @return list<string> The words.
 */
function tokenize(string $text, bool $keepSpace = false): array
{
    if (!function_exists('helper')) {
        function helper() {}
    }
    return explode(' ', $text);
}

/**
 * Makes a lexer.
 */
function lexer(string $text): Lexer
{
    return new Lexer($text);
}

$closure = function ($x) use ($text) {
    return $x;
};

if (!function_exists('conditional')) {
    function conditional() {}
}
