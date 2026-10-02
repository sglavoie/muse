package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
)

func main() {
	configureUsage()

	// Handle global flags before a subcommand is selected. Once the first
	// non-flag token appears, later tokens belong to that command and should
	// be validated by the command-specific argument checks below.
	if runGlobalFlag(detectGlobalFlag(os.Args[1:]), os.Stdout) {
		return
	}

	// Hidden layout-testing harness. Dispatched before flag parsing and
	// deliberately absent from configureUsage() so it never appears in normal
	// `muse --help` output.
	if len(os.Args) > 1 && os.Args[1] == "tui-test" {
		os.Exit(runTUITestCLI(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	// Hidden build-info report for muse.app's binary resolver, which needs a
	// build time to prefer the newer of two installed binaries.
	if len(os.Args) > 1 && os.Args[1] == "build-info" {
		if err := validateNoCommandArgs(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "muse: %v\n", err)
			os.Exit(1)
		}
		printBuildInfo(os.Stdout)
		return
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

	controlPath := flag.String("control", "", "")
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "muse: unknown command %q\n", flag.Arg(0))
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals()...)

	if err := runTUI(ctx, tuiOptions{ControlPath: *controlPath}); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(exitCodeFor(err))
	}
	cancel()
}

// configureUsage points the flag package's usage hook, which runs on a usage
// error, at stderr. An explicit --help prints the same text to stdout instead.
func configureUsage() {
	flag.Usage = func() { printUsage(os.Stderr) }
}

// printUsage writes the help text. The flags are listed by hand: the global
// flags are detected by detectGlobalFlag rather than registered with the flag
// package, and the --control line has a fixed shape a host app parses.
func printUsage(w io.Writer) {
	var b strings.Builder
	b.WriteString("Usage: muse [command]\n\nCommands:\n")
	for _, c := range localCommands {
		fmt.Fprintf(&b, "  %-14s %s\n", c.Name, c.Desc)
	}
	for _, a := range remoteActions {
		fmt.Fprintf(&b, "  %-14s %s\n", a.Name, a.Desc)
	}
	b.WriteString("\nFlags:\n")
	fmt.Fprintf(&b, "  %-14s %s\n", "-h, --help", "Show this help and exit")
	fmt.Fprintf(&b, "  %-14s %s\n", "--version", "Print the version and exit")
	// muse.app probes for this line, taking its first token as the flag name.
	fmt.Fprintf(&b, "  %-14s %s\n", "--control PATH", "Serve a JSON control socket at PATH for a host app")
	_, _ = io.WriteString(w, b.String())
}

// runGlobalFlag handles --version and --help, writing to stdout, and reports
// whether f was one of them.
func runGlobalFlag(f globalFlag, stdout io.Writer) bool {
	switch f {
	case globalFlagVersion:
		_, _ = fmt.Fprintln(stdout, resolveVersion())
		return true
	case globalFlagHelp:
		printUsage(stdout)
		return true
	}
	return false
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
	{"now-playing", "Notify the current track via a desktop notification", runNowPlaying},
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
