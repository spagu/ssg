<?php
namespace Acme\Textkit;

use Acme\Textkit\Lexer\Lexer as L;
use Acme\Textkit\Token\Kind;

/**
 * A parsed document.
 *
 * @internal
 */
readonly class Doc
{
    /**
     * Builds a document.
     *
     * @param list<Kind> $kinds The kinds found.
     * @param string $title Shown as the heading.
     */
    private function __construct(
        public array $kinds,
        public string $title = 'Untitled',
        public private(set) ?L $lexer = null,
    ) {
    }

    /**
     * Makes an empty document.
     *
     * @deprecated Use {@link Doc::of()} instead.
     */
    public static function empty(): self
    {
        return new self([]);
    }

    #[\Deprecated(message: "use title instead", since: "2.1")]
    public function heading(): string
    {
        return $this->title;
    }

    /**
     * A document from a lexer.
     *
     * @param L|null $lx The lexer, if any.
     */
    public static function of(?L $lx = null, int|string ...$parts): static
    {
        return new static([]);
    }

    /** @private */
    public function internalOnly(): void
    {
    }
}

/**
 * Settings for {@see Doc}.
 */
final class Options
{
    public string $name = 'a';

    public function name(): string
    {
        return $this->name;
    }
}
