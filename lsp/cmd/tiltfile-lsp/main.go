// Command tiltfile-lsp is a language server for Tiltfiles.
//
// It speaks LSP over stdin and stdout, so any client can drive it:
//
//	tiltfile-lsp
//
// The VS Code extension bundles this binary. A standalone build exists so a
// bug report can name a version, and so other editors can use it.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/owenrumney/go-lsp/server"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/builtins"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/handler"
)

// version is set by the linker at release time, and stays "dev" otherwise.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		// The Tilt version is the API the builtin table describes, which is
		// the first thing to check against a report about a missing builtin.
		fmt.Printf("tiltfile-lsp %s (Tiltfile API from Tilt %s)\n",
			version, builtins.TiltVersion)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "tiltfile-lsp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv := server.NewServer(handler.New(version))
	return srv.Run(ctx, server.RunStdio())
}
