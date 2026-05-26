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
			name: "help before command",
			args: []string{"-h", "next"},
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
