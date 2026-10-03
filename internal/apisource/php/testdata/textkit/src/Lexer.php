<?php

declare(strict_types=1);

namespace Acme\Textkit\Lexer;

use Acme\Textkit\Token\Kind;
use Acme\Textkit\Token\{Tokenizer, Positioned as Located};
use function Acme\Textkit\tokenize;
use Countable;

/**
 * The common ground of every lexer.
 *
 * @see Lexer
 */
abstract class Base
{
    /** Default size of the read buffer. */
    public const BUFFER = 4096;

    /**
     * Reads the next kind of token.
     *
     * @return Kind|null The kind, or null at the end.
     */
    abstract public function next(): ?Kind;

    protected function helper(): void
    {
    }
}

/**
 * Splits source text into tokens.
 *
 * Use {@see Base::next()} to step through them, or read {@link Kind}.
 *
 * @since 2.0.0
 * @example
 * $lx = new Lexer('a b');
 * echo count($lx);
 */
#[\Attribute(\Attribute::TARGET_CLASS)]
final class Lexer extends Base implements Countable, Tokenizer
{
    use Located;

    const MAX = 32, MIN = 1;

    private const SECRET = 'x';

    /** @var string[] Words seen so far. */
    public array $words = [];

    /**
     * How deep the lexer is nested.
     */
    public static int $depth = 0;

    public ?Kind $last = null, $first = null;

    private $hidden;

    var $legacy = '}';

    public function __construct(
        public readonly string $source,
        #[\SensitiveParameter] private string $key = '',
        protected int $offset = 0,
        bool $plain = false,
    ) {
        $template = <<<EOT
            function fake() { return "}"; }
            {$this->source}
            EOT;
        $raw = <<<'RAW'
        class Nope { }
        RAW;
        $s = "a } b \" c";
        $t = 'it\'s } here';
        # a hash comment with { brace
        // a line comment with } brace
        /* block { comment */
    }

    /**
     * Reads the next kind.
     *
     * @return Kind|null
     */
    public function next(): ?Kind
    {
        return null;
    }

    public function count(): int
    {
        return \count($this->words);
    }

    /**
     * Parses source text.
     *
     * @param string $src The text to read.
     * @param array<string, mixed> $opts Options, by name.
     * @throws \InvalidArgumentException When the text is empty.
     * @return static A lexer over the text.
     */
    public static function parse(string $src, array $opts = []): static
    {
        $fn = function () use ($src) { return $src; };
        return new static($src);
    }

    /**
     * Returns the words.
     *
     * @param int|null $limit Most words to return.
     * @param string ...$skip Words to leave out.
     * @return string[]
     */
    public function words($limit, string &...$skip)
    {
        return [];
    }

    final public function &reference(Kind&Countable $k, \Acme\Textkit\Lexer\Base|int|null $b): self
    {
        return $this;
    }

    private function secret(): void {}

    protected static function guarded(): void {}
}
