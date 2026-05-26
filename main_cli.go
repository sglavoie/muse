package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	configureUsage()

	// Handle global flags before a subcommand is selected. Once the first
	// non-flag token appears, later tokens belong to that command and should
	// be validated by the command-specific argument checks below.
	switch detectGlobalFlag(os.Args[1:]) {
	case globalFlagVersion:
		fmt.Println(Version)
		return
	case globalFlagHelp:
		flag.Usage()
		return
	}

	// Handle subcommands before flag parsing.
	if len(os.Args) > 1 && os.Args[1] == "check" {
		if err := validateNoCommandArgs(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "muse: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runCheck())
	}

	if len(os.Args) > 1 {
		if isKnownAction(os.Args[1]) {
			if err := validateNoCommandArgs(os.Args[1], os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "muse: %v\n", err)
				os.Exit(1)
			}
			if err := runRemoteAction(os.Args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "muse %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			return
		}
	}

	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "muse: unknown command %q\n", flag.Arg(0))
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := runTUI(ctx); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}
	cancel()
}

func configureUsage() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: muse [command] [flags]\n\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  %-14s %s\n", "check", "Validate config and prerequisites, then exit")
		for _, a := range remoteActions {
			fmt.Fprintf(os.Stderr, "  %-14s %s\n", a.Name, a.Desc)
		}
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
}

func validateNoCommandArgs(command string, args []string) error {
	if len(args) == 0 {
		return nil
	}
	// Accept a bare "--" end-of-options marker for parity with common CLI
	// conventions when a command takes no arguments.
	if args[0] == "--" {
		args = args[1:]
		if len(args) == 0 {
			return nil
		}
	}
	word := "argument"
	if len(args) > 1 {
		word = "arguments"
	}
	return fmt.Errorf("%s does not accept %s: %s", command, word, strings.Join(args, " "))
}

type globalFlag int

const (
	globalFlagNone globalFlag = iota
	globalFlagVersion
	globalFlagHelp
)

func detectGlobalFlag(args []string) globalFlag {
	for _, arg := range args {
		if arg == "--" {
			return globalFlagNone
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return globalFlagNone
		}
		switch arg {
		case "--version", "-version":
			return globalFlagVersion
		case "--help", "-help", "-h":
			return globalFlagHelp
		default:
			return globalFlagNone
		}
	}
	return globalFlagNone
}
