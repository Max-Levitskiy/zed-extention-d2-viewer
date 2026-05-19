package main

import (
	"flag"
	"fmt"
	"os"
)

var Version = "0.0.0"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *versionFlag {
		fmt.Println(Version)
		os.Exit(0)
	}
	// Full LSP loop wired in Task 6.
	fmt.Fprintln(os.Stderr, "d2-lsp: LSP loop not yet wired (see Task 6)")
	os.Exit(2)
}
