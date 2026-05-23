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
