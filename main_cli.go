package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
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

	// Hidden layout-testing harness. Dispatched before flag parsing and
	// deliberately absent from configureUsage() so it never appears in normal
	// `muse --help` output.
	if len(os.Args) > 1 && os.Args[1] == "tui-test" {
		os.Exit(runTUITestCLI(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	// Handle local subcommands before flag parsing. These run entirely in this
	// process, unlike remoteActions which are relayed to a running instance
	// over the control socket.
	if len(os.Args) > 1 {
		if cmd, ok := lookupLocalCommand(os.Args[1]); ok {
			if err := validateNoCommandArgs(os.Args[1], os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "muse: %v\n", err)
				os.Exit(1)
			}
			os.Exit(cmd.Run())
		}
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
		for _, c := range localCommands {
			fmt.Fprintf(os.Stderr, "  %-14s %s\n", c.Name, c.Desc)
		}
		for _, a := range remoteActions {
			fmt.Fprintf(os.Stderr, "  %-14s %s\n", a.Name, a.Desc)
		}
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
}

// localCommand is a subcommand handled in-process. Deliberately kept separate
// from remoteActions: that slice is the control-socket vocabulary, and adding
// a local command to it would make the TUI accept it over muse.ctrl and
// dispatch a RemoteActionMsg nothing handles.
type localCommand struct {
	Name string
	Desc string
	Run  func() int
}

// localCommands is the authoritative list of in-process subcommands. It drives
// both dispatch and help text, so adding an entry here is the only change
// required.
var localCommands = []localCommand{
	{"check", "Validate config and prerequisites, then exit", runCheck},
	{"now-playing", "Notify the current track via macOS notification", runNowPlaying},
}

// lookupLocalCommand returns the local command registered under name.
func lookupLocalCommand(name string) (localCommand, bool) {
	for _, c := range localCommands {
		if c.Name == name {
			return c, true
		}
	}
	return localCommand{}, false
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
		name, ok := globalBoolFlagName(arg)
		if !ok {
			return globalFlagNone
		}
		switch name {
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

func globalBoolFlagName(arg string) (string, bool) {
	name, value, hasValue := strings.Cut(arg, "=")
	if !hasValue {
		return name, true
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil || !enabled {
		return "", false
	}
	return name, true
}
