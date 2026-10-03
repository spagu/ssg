package textkit_test

import (
	"fmt"
	"strings"

	"example.com/textkit"
)

func Example() {
	fmt.Println("textkit")
	// Output: textkit
}

func ExampleNewLexer() {
	l := textkit.NewLexer(strings.NewReader("a b"))
	_ = l
}

func ExampleLexer_Next() {
	l := textkit.NewLexer(strings.NewReader("a"))
	// the first token
	fmt.Println(l.Next().Text)
	// Output:
	// a
}
