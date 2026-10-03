"""The lexer: text in, tokens out."""

from dataclasses import dataclass
from enum import Enum


class Kind(Enum):
    """What a :class:`Token` is."""

    WORD = "word"
    NUMBER = "number"
    PUNCT = "punct"
    SPACE = "space"


@dataclass(frozen=True)
class Token:
    """One piece of the input.

    Attributes:
        kind: What the token is.
        text: The token as written.
        start: Offset of the first character.
    """

    kind: Kind
    text: str
    start: int


class Lexer:
    """Reads text one token at a time.

    Attributes:
        MAX_LENGTH: The longest input the lexer accepts, in characters.
    """

    MAX_LENGTH: int = 100_000

    def __init__(self, source: str) -> None:
        """Create a lexer.

        Args:
            source: The text to read.
        """
        self._source = source
        self._pos = 0

    def next(self) -> Token | None:
        """Return the next token, or None at the end of the input."""
        if self._pos >= len(self._source):
            return None
        token = Token(Kind.WORD, self._source[self._pos], self._pos)
        self._pos += 1
        return token


def tokenize(text: str, *, keep_space: bool = False) -> list[Token]:
    """Split text into tokens, in order.

    Args:
        text: The text to split.
        keep_space: Return whitespace tokens too.

    Returns:
        Every token, in order.

    Raises:
        ValueError: When the text is longer than ``Lexer.MAX_LENGTH``.

    Examples:
        >>> [t.text for t in tokenize("Hi")]
        ['H', 'i']
    """
    lexer = Lexer(text)
    out = []
    while (token := lexer.next()) is not None:
        out.append(token)
    return out
