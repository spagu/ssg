"""Lexical analysis for textkit.

The lexer turns text into :class:`Token` values.
"""

from __future__ import annotations

from collections.abc import Iterable

from .formats import Style

MAX_DEPTH: int = 32
"""How deep nested groups may go."""

FAKE = "def fake(): pass"

_PRIVATE = 1

TEMPLATE = """
class X:
    def method(self): ...
"""


class Token:
    """One token of the input.

    Attributes:
        kind: The token kind.
        text: The token text.
    """

    kind: str
    text: str = ""

    def __init__(self, kind: str, text: str = "") -> None:
        self.kind = kind
        self.text = text


class Lexer(Iterable[Token]):
    """Splits text into tokens.

    Args:
        text: The source text.
        style (Style): How to render tokens.

    Example:
        >>> lx = Lexer("a b")
        >>> [t.text for t in lx]
        ['a', 'b']
    """

    def __init__(self, text: str, style: Style | None = None, *, strict: bool = False) -> None:
        self._text = text
        self._pos = 0

    @property
    def position(self) -> int:
        """The current offset."""
        return self._pos

    @position.setter
    def position(self, value: int) -> None:
        self._pos = value

    @property
    def text(self) -> str:
        """The source text, read-only."""
        return self._text

    def next(self) -> Token:
        """Returns the next token.

        Returns:
            Token: The token at the current
                position.

        Raises:
            StopIteration: At the end of the input.
        """

    @staticmethod
    def split(text: str, /, sep: str = " ") -> list[str]:
        """Splits text on a separator."""
        return text.split(sep)

    @classmethod
    def from_file(cls, path: str, *args: str, **kwargs: int) -> "Lexer":
        """Reads a lexer from a file."""

    async def fetch(self, url: str) -> bytes:
        """Fetches remote text."""

    def _helper(self): pass

    def __iter__(self): ...


def tokenize(text: str, *, keep_space: bool = False) -> list[Token]:
    """Tokenizes text.

    Args:
        text: The text to split.
        keep_space: Keep whitespace
            tokens.

    Returns:
        list[Token]: The tokens, in order.

    Raises:
        ValueError: If text is not a string.

    Examples:
        >>> tokenize("a b")
        [Token('word', 'a'), Token('word', 'b')]
    """
    s = f"{text!r:>{10}} {'nested'} {{def inner(): pass}}"
    return [s]
