package textkit_test

import (
	"fmt"

	"example.com/textkit"
)

func ExampleTokenize() {
	fmt.Println(len(textkit.Tokenize("Hi", false)))
	// Output: 2
}
