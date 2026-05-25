package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sglavoie/muse/internal/config"
)

// runCheck validates the muse configuration and prerequisites without
// launching the full TUI. It prints warnings and errors to stderr and returns
// 0 on success or 1 if any check fails.
func runCheck() int {
	dirs, err := config.ResolveDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve dirs: %v\n", err)
		return 1
	}

	loadResult, err := config.Load(dirs.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		return 1
	}

	for _, w := range loadResult.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	exitCode := 0

	configPath := config.ConfigPath(dirs.Config)
	if err := config.ValidateMusicDirs(loadResult.Config.MusicDirectories, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		exitCode = 1
	}

	if err := config.CheckPrereqs(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		exitCode = 1
	}

	if exitCode == 0 {
		fmt.Println("ok")
	}
	return exitCode
}
