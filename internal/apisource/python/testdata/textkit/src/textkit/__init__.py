"""Text processing toolkit.

Import what you need from here: :class:`Lexer` reads text.
"""
from .lexer import Lexer, tokenize as tokenize, Token
from .formats import *
from . import util

__all__ = ["Lexer", "Token", "tokenize", "Style", "util", "missing"]
__version__ = "1.0.0"
