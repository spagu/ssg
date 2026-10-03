"""Tokenize and format text.

Start with :func:`tokenize`, or read tokens one by one with :class:`Lexer`.
"""

from .lexer import Kind, Lexer, Token, tokenize

__all__ = ["Kind", "Lexer", "Token", "tokenize"]
