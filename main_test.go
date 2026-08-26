package main

import (
	"strings"
	"testing"
)

func TestValidateNoCommandArgs_AllowsNoArgs(t *testing.T) {
	if err := validateNoCommandArgs("next", nil); err != nil {
		t.Fatalf("validateNoCommandArgs: %v", err)
	}
}

func TestValidateNoCommandArgs_RejectsTrailingArg(t *testing.T) {
	err := validateNoCommandArgs("next", []string{"extra"})
	if err == nil {
		t.Fatal("expected error for trailing arg")
	}
	if got := err.Error(); !strings.Contains(got, "next does not accept argument: extra") {
		t.Fatalf("error = %q", got)
	}
}

func TestValidateNoCommandArgs_RejectsMultipleTrailingArgs(t *testing.T) {
	err := validateNoCommandArgs("check", []string{"one", "two"})
	if err == nil {
		t.Fatal("expected error for trailing args")
	}
	if got := err.Error(); !strings.Contains(got, "check does not accept arguments: one two") {
		t.Fatalf("error = %q", got)
	}
}

func TestValidateNoCommandArgs_AllowsBareDoubleDash(t *testing.T) {
	if err := validateNoCommandArgs("check", []string{"--"}); err != nil {
		t.Fatalf("validateNoCommandArgs should allow bare double dash: %v", err)
	}
}

func TestValidateNoCommandArgs_RejectsArgsAfterDoubleDash(t *testing.T) {
	err := validateNoCommandArgs("next", []string{"--", "extra"})
	if err == nil {
		t.Fatal("expected error for trailing arg after double dash")
	}
	if got := err.Error(); !strings.Contains(got, "next does not accept argument: extra") {
		t.Fatalf("error = %q", got)
	}
}

func TestDetectGlobalFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want globalFlag
	}{
		{
			name: "version before command",
			args: []string{"--version", "next"},
			want: globalFlagVersion,
		},
		{
			name: "version true before command",
			args: []string{"--version=true", "next"},
			want: globalFlagVersion,
		},
		{
			name: "help before command",
			args: []string{"-h", "next"},
			want: globalFlagHelp,
		},
		{
			name: "help true before command",
			args: []string{"-h=true", "next"},
			want: globalFlagHelp,
		},
		{
			name: "long single-dash help before command",
			args: []string{"-help", "next"},
			want: globalFlagHelp,
		},
		{
			name: "command owns trailing help",
			args: []string{"next", "--help"},
			want: globalFlagNone,
		},
		{
			name: "unknown command owns trailing help",
			args: []string{"not-a-command", "--help"},
			want: globalFlagNone,
		},
		{
			name: "unknown leading flag left to flag package",
			args: []string{"--not-a-real-flag", "--help"},
			want: globalFlagNone,
		},
		{
			name: "false version flag left to flag package",
			args: []string{"--version=false", "next"},
			want: globalFlagNone,
		},
		{
			name: "invalid help flag value left to flag package",
			args: []string{"--help=maybe", "next"},
			want: globalFlagNone,
		},
		{
			name: "double dash stops global flag parsing",
			args: []string{"--", "--help"},
			want: globalFlagNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectGlobalFlag(tt.args); got != tt.want {
				t.Fatalf("detectGlobalFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestLocalCommandsDisjointFromRemoteActions guards the boundary between the
// two subcommand tables. remoteActions is the control-socket vocabulary: an
// entry there makes the TUI accept the name over muse.ctrl and dispatch a
// RemoteActionMsg. Local commands run in-process and have no such handler, so
// listing one in both places would produce a silently ignored message.
func TestLocalCommandsDisjointFromRemoteActions(t *testing.T) {
	for _, c := range localCommands {
		if isKnownAction(c.Name) {
			t.Errorf("local command %q is also a remote action; it must not be", c.Name)
		}
	}
	for _, a := range remoteActions {
		if _, ok := lookupLocalCommand(a.Name); ok {
			t.Errorf("remote action %q is also a local command; it must not be", a.Name)
		}
	}
}

func TestLookupLocalCommand(t *testing.T) {
	for _, name := range []string{"check", "now-playing"} {
		cmd, ok := lookupLocalCommand(name)
		if !ok {
			t.Errorf("lookupLocalCommand(%q): not found", name)
			continue
		}
		if cmd.Name != name {
			t.Errorf("lookupLocalCommand(%q).Name = %q", name, cmd.Name)
		}
		if cmd.Run == nil {
			t.Errorf("lookupLocalCommand(%q).Run is nil", name)
		}
		if cmd.Desc == "" {
			t.Errorf("lookupLocalCommand(%q).Desc is empty", name)
		}
	}

	if _, ok := lookupLocalCommand("tui-test"); ok {
		t.Error("tui-test must stay hidden from localCommands")
	}
	if _, ok := lookupLocalCommand("nope"); ok {
		t.Error("lookupLocalCommand(\"nope\") = true, want false")
	}
}
