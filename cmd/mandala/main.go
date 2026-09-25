package main

import (
	"fmt"
	"os"

	"github.com/cottondesu/mandala/internal/mandala"
)

func main() {
	start, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mandala: E_IO: get working directory: %v\n", err)
		os.Exit(2)
	}
	os.Exit(mandala.Run(os.Args[1:], start, os.Stdout, os.Stderr))
}
