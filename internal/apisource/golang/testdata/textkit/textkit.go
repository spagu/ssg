// Package textkit splits text into tokens and formats them again.
//
// Start with [NewLexer] and call [Lexer.Next]; see [lexer.Token] for what
// comes out, and [io.Reader] for the input.
package textkit

import (
	"errors"
	"io"

	"example.com/textkit/lexer"
)

// MaxDepth is how deep nested blocks may go.
const MaxDepth = 32

// Errors the lexer returns.
var (
	ErrClosed = errors.New("textkit: the lexer is closed and cannot be read again by anyone at all, ever")
	// ErrLimit means MaxDepth was reached.
	ErrLimit error
)

// Lexer reads tokens from a source.
//
// It is not safe for concurrent use.
type Lexer struct {
	// Src is the input.
	Src          io.Reader
	Line, Column int // position of the next token
	Last         lexer.Token
	state        int
}

// NewLexer returns a lexer reading src.
func NewLexer(src io.Reader, opts ...Option) *Lexer { return &Lexer{Src: src} }

// Next returns the next token.
func (l *Lexer) Next() lexer.Token { return l.Last }

// Close stops the lexer.
func (l Lexer) Close() error { return nil }

func (l *Lexer) reset() {}

// Option configures a [Lexer].
type Option func(*Lexer)

// Apply runs the option.
func (o Option) Apply(l *Lexer) { o(l) }

// Source is anything a lexer can read.
type Source interface {
	io.Reader
	// Name says where the text comes from.
	Name() string
	Peek(n int) ([]byte, error)
}

// Level is how much the lexer reports.
type Level int

// Levels, quietest first.
const (
	// Quiet reports nothing.
	Quiet  Level = iota
	Normal       // the default
	Loud
)

// DefaultLevel is used when none is set.
var DefaultLevel Level = Normal

// Number is a type set.
type Number interface {
	~int | ~float64
}

// Sum adds up values.
func Sum[T Number](values ...T) (total T) { return }

// Set is a set of comparable values.
type Set[T comparable] struct {
	Items map[T]struct{}
}

// Add puts v in the set.
func (s *Set[T]) Add(v T) {}

// Pair holds two values.
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

// Swap returns the pair reversed.
func (p Pair[K, V]) Swap() (V, K) { return p.Value, p.Key }

// Doc embeds a lexer.
type Doc struct {
	*Lexer
	Tokens chan lexer.Token
	Index  map[string][]*Lexer
}

// Point is an alias of a struct.
type Point = struct{ X, Y int }

// Any is an alias of an interface.
type Any = interface{}

// Mode is the old name of Level.
type Mode = Level

// Split cuts text into words.
//
// Deprecated: use [Lexer.Next] instead.
func Split(text string) []string { return nil }

// Join puts words together.
//
//	Join([]string{"a", "b"}, " ") // "a b"
//
// # Notes
//
// It panics on [lexer] errors; see [the tokens] and [errors.New].
//
// [the tokens]: https://example.com/tokens
func Join(words []string, _ string) string { return "" }

// Pipe connects two lexers.
func Pipe(chan<- int, <-chan int) {}
