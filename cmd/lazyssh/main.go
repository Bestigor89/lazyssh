package main

import (
	"fmt"
	"os"

	"github.com/Bestigor89/lazyssh/internal/config"
	"github.com/Bestigor89/lazyssh/internal/ui"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	// Handle --version / -v before starting the TUI.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println("lazyssh", version)
			os.Exit(0)
		}
	}

	ui.Version = version

	store, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lazyssh: load config: %v\n", err)
		os.Exit(1)
	}

	app := ui.NewApp(store)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lazyssh: %v\n", err)
		os.Exit(1)
	}
}
