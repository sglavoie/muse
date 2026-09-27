package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
)

// runCheck validates the muse configuration and prerequisites without
// launching the full TUI. It prints warnings and errors to stderr and returns
// 0 on success or 1 if any check fails.
func runCheck() int {
	discardLogs()

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

	ctx := context.Background()

	prereqWarnings, err := config.CheckPrereqs(ctx)
	for _, w := range prereqWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if err != nil {
		printErrors(err)
		exitCode = 1
	}

	// Report the backend a scan would actually use. CheckPrereqs already warns
	// about the degraded path, so this line stays purely informational and
	// never changes the exit code on its own: a resolved backend is a success,
	// and an unresolved one has already been reported as a fatal prereq error.
	if _, backend, selectErr := library.SelectExtractor(ctx); selectErr == nil {
		fmt.Fprintf(os.Stderr, "metadata backend: %s\n", backend)
	}

	if exitCode == 0 {
		fmt.Println("ok")
	}
	return exitCode
}

// printErrors writes err to stderr with an "error:" prefix. CheckPrereqs joins
// independent failures (mpv and the metadata backend) with errors.Join, so each
// joined error gets its own prefixed entry instead of the second one running on
// from the first one's install instructions.
func printErrors(err error) {
	// A direct assertion, not errors.As: only a join at the top is split, so a
	// joined error wrapped with extra context is printed whole, context intact.
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}
	for _, e := range joined.Unwrap() {
		fmt.Fprintf(os.Stderr, "error: %v\n", e)
	}
}

// discardLogs silences slog for the one-shot commands, which never call
// config.InitLogging. Without it, config.Load's validation warnings reach
// stderr twice: once through slog's default handler and once as the command's
// own "warning:" lines, which carry the same information in a friendlier form.
func discardLogs() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
}
