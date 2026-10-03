"""Utilities.

See :mod:`textkit` and [`Lexer`] for more.
"""
import sys
from typing import TypeAlias
from typing_extensions import deprecated

from .lexer import Lexer, Token

__all__ = ["first", "Pair", "Number", "old_join", "Box", "Lexer"]

Number: TypeAlias = int | float

type Pair[T] = tuple[T, T]


def first[T](items: list[T], default: T | None = None) -> T | None:
    """Returns the first item.

    :param items: The items.
    :type items: list
    :param default: What to return for no items.
    :returns: The first item or the default.
    :rtype: T
    :raises IndexError: Never.

    Example::

        first([1, 2])
    """


@deprecated("Use str.join instead.")
def old_join(parts, sep=","):
    """Joins parts.

    :param list parts: The parts.
    :param str sep: The separator.
    :rtype: str
    """


class Box[T]:
    """A box holding one value of type ``T``.

    .. versionadded:: 1.1
    """

    def __init__(self, value: T):
        """Makes a box.

        :param value: The value.
        """

    def get(self) -> T:
        """Returns the value. See :meth:`Box.get`."""


def _private():
    pass
