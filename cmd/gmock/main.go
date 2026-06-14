// Package main is the gmock CLI entry point.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "gmock:", err)
		os.Exit(1)
	}
}