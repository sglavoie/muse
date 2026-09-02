package uitest

import (
	"testing"
)

func hasIssue(issues []UITestIssue, kind UITestIssueKind) *UITestIssue {
	for i := range issues {
		if issues[i].Kind == kind {
			return &issues[i]
		}
	}
	return nil
}

func TestPlainScreenStripsANSI(t *testing.T) {
	raw := "\x1b[31mhello\x1b[0m world"
	if got := plainScreen(raw); got != "hello world" {
		t.Errorf("PlainScreen = %q, want %q", got, "hello world")
	}
}

func TestDetectLayoutIssuesClean(t *testing.T) {
	plain := "line one\nline two\nline three"
	issues := detectLayoutIssues(plain, plain, 20, 10)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %+v", issues)
	}
}

func TestDetectWidthOverflow(t *testing.T) {
	plain := "short\nthis line is way too wide"
	issues := detectLayoutIssues(plain, plain, 10, 10)
	iss := hasIssue(issues, issueWidthOverflow)
	if iss == nil {
		t.Fatalf("expected width_overflow, got %+v", issues)
	}
	if iss.Line != 2 {
		t.Errorf("width_overflow line = %d, want 2", iss.Line)
	}
	if iss.Limit != 10 || iss.Actual <= 10 {
		t.Errorf("width_overflow actual/limit = %d/%d", iss.Actual, iss.Limit)
	}
}

func TestDetectWidthOverflowWideRunes(t *testing.T) {
	// CJK glyphs occupy two display columns each; five of them exceed width 8.
	plain := "日本語テス"
	issues := detectLayoutIssues(plain, plain, 8, 5)
	if hasIssue(issues, issueWidthOverflow) == nil {
		t.Errorf("expected width_overflow for wide runes, got %+v", issues)
	}
}

func TestDetectHeightOverflow(t *testing.T) {
	plain := "a\nb\nc\nd\ne"
	issues := detectLayoutIssues(plain, plain, 40, 3)
	iss := hasIssue(issues, issueHeightOverflow)
	if iss == nil {
		t.Fatalf("expected height_overflow, got %+v", issues)
	}
	if iss.Actual != 5 || iss.Limit != 3 {
		t.Errorf("height_overflow actual/limit = %d/%d, want 5/3", iss.Actual, iss.Limit)
	}
}

func TestDetectTabCharacter(t *testing.T) {
	plain := "col1\tcol2"
	issues := detectLayoutIssues(plain, plain, 40, 10)
	iss := hasIssue(issues, issueTabCharacter)
	if iss == nil {
		t.Fatalf("expected tab_character, got %+v", issues)
	}
	if iss.Line != 1 {
		t.Errorf("tab_character line = %d, want 1", iss.Line)
	}
}

func TestDetectBlankScreen(t *testing.T) {
	for _, plain := range []string{"", "   ", "\n\n  \n"} {
		issues := detectLayoutIssues(plain, plain, 40, 10)
		if hasIssue(issues, issueBlankScreen) == nil {
			t.Errorf("expected blank_screen for %q, got %+v", plain, issues)
		}
	}
}

func TestDetectInvalidUTF8(t *testing.T) {
	raw := "valid\xff\xfebytes"
	issues := detectLayoutIssues(raw, plainScreen(raw), 40, 10)
	if hasIssue(issues, issueInvalidUTF8) == nil {
		t.Errorf("expected invalid_utf8, got %+v", issues)
	}
}

func TestDetectDisablesChecksOnZeroBounds(t *testing.T) {
	plain := "a very very very long single line"
	issues := detectLayoutIssues(plain, plain, 0, 0)
	if hasIssue(issues, issueWidthOverflow) != nil {
		t.Errorf("width check should be disabled at width 0: %+v", issues)
	}
	if hasIssue(issues, issueHeightOverflow) != nil {
		t.Errorf("height check should be disabled at height 0: %+v", issues)
	}
}

func TestTrailingNewlineNotCountedAsLine(t *testing.T) {
	plain := "a\nb\nc\n"
	issues := detectLayoutIssues(plain, plain, 40, 3)
	if hasIssue(issues, issueHeightOverflow) != nil {
		t.Errorf("trailing newline should not push over height: %+v", issues)
	}
}
