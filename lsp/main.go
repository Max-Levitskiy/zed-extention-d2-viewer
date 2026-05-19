package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/max-levitskiy/zed-extention-d2-viewer/lsp/internal/server"
)

// Version is a var (not const) so the release pipeline can rewrite it with
// `-ldflags "-X main.Version=<tag>"` — `-X` only works on string vars.
var Version = "0.0.0"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *versionFlag {
		fmt.Println(Version)
		os.Exit(0)
	}
	if err := server.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "d2-lsp:", err)
		os.Exit(1)
	}
}
