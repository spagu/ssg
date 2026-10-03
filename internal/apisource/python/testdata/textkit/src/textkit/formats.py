"""Output formats.

Formats decide how tokens are rendered.
"""
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum, auto
from typing import ClassVar, Optional, Protocol, Union, overload


class Style(Enum):
    """Rendering styles."""

    PLAIN = 1
    """No markup."""
    HTML = "html"
    ANSI = auto()

    def describe(self) -> str:
        """Describes the style."""


class Renderer(Protocol):
    """Something that renders tokens."""

    def render(self, text: str) -> str:
        """Renders text.

        Parameters
        ----------
        text : str
            The text to render.

        Returns
        -------
        str
            The rendered text.
        """
        ...


class Base(ABC):
    """An abstract base."""

    @abstractmethod
    def run(self) -> None:
        """Runs."""


@dataclass(frozen=True)
class Options:
    """Rendering options.

    Parameters
    ----------
    width : int
        Line width.
    style : Style, optional
        The style.
    """

    width: int = 80
    style: Style = Style.PLAIN
    tags: list[str] = field(default_factory=list)
    cache: ClassVar[dict] = {}
    secret: int = field(init=False, default=0)


def render(text, options: Optional[Options] = None, *extra: Union[str, int]):
    """Renders text with options.

    .. deprecated:: 1.2
       Use :func:`render_all` instead.

    Parameters
    ----------
    text : str
        The text.
    options : Options, optional
        How to render.
    *extra : str
        Extra parts.

    Returns
    -------
    str
        The rendered text.

    Raises
    ------
    ValueError
        If the options are invalid.

    See Also
    --------
    tokenize : Splits text.

    Examples
    --------
    >>> render("a")
    'a'
    """


@overload
def parse(x: int) -> int: ...
@overload
def parse(x: str) -> str: ...
def parse(x):
    """Parses x."""
    return x
