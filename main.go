package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *version {
		fmt.Println(Version)
		os.Exit(0)
	}

	dirs, err := config.ResolveDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "muse: resolve dirs: %v\n", err)
		os.Exit(1)
	}

	// Init logging before loading config so any config warnings are captured.
	if err := config.InitLogging(dirs.State, slog.LevelInfo); err != nil {
		fmt.Fprintf(os.Stderr, "muse: init logging: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("log:", config.LogPath())

	configPath := filepath.Join(dirs.Config, "config.toml")
	cfg, configCreated, err := config.Load(dirs.Config)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		fmt.Fprintf(os.Stderr, "muse: load config: %v\n", err)
		os.Exit(1)
	}
	_ = cfg // cfg will be used when wiring real MPV flags and music directory

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps := app.Deps{
		LaunchMPV: func(ctx context.Context, socketPath string) error {
			slog.Info("startup: mpv launch (stub)")
			return nil
		},
		RunScan: func(ctx context.Context, database *db.DB) error {
			slog.Info("startup: library scan (stub)")
			return nil
		},
	}

	result, err := app.Run(ctx, dirs, deps)
	if err != nil {
		slog.Error("startup failed", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}

	model := tui.New(config.LogPath(), configPath, configCreated)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		slog.Error("TUI error", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		// Still run shutdown to clean up resources.
		app.Shutdown(app.ShutdownDeps{Lockfile: result.Lockfile, SocketPath: result.SocketPath})
		os.Exit(1)
	}

	app.Shutdown(app.ShutdownDeps{Lockfile: result.Lockfile, SocketPath: result.SocketPath})
}
