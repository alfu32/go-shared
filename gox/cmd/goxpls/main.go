package main

import (
	"fmt"
	"os"

	"github.com/alfu32/go-shared/gox/lsp"
)

func main() {
	if err := goxpls.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
