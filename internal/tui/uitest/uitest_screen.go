package uitest

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// PlainScreen strips ANSI escape sequences from rendered terminal content,
// yielding the visible text used for width checks, blank detection, and
// substring assertions.
func PlainScreen(raw string) string {
	return ansi.Strip(raw)
}

// screenLines splits rendered content into display lines, normalizing CRLF and
// bare CR to LF first. A single trailing newline is not treated as an extra
// empty line, matching how a terminal renders content.
func screenLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// DetectLayoutIssues inspects rendered content against the simulated terminal
// dimensions and returns any layout problems found. raw is the original
// (possibly ANSI-styled) content and is used only for the UTF-8 validity check;
// all width/height/tab/blank analysis runs on the ANSI-stripped plain text.
//
// Width and height bounds of zero (or negative) disable the corresponding
// overflow check. Line numbers in returned issues are 1-based.
func DetectLayoutIssues(raw, plain string, width, height int) []UITestIssue {
	var issues []UITestIssue

	if !utf8.ValidString(raw) {
		issues = append(issues, UITestIssue{
			Kind:   IssueInvalidUTF8,
			Detail: "screen content is not valid UTF-8",
		})
	}

	if strings.TrimSpace(plain) == "" {
		issues = append(issues, UITestIssue{Kind: IssueBlankScreen})
	}

	lines := screenLines(plain)
	if height > 0 && len(lines) > height {
		issues = append(issues, UITestIssue{
			Kind:   IssueHeightOverflow,
			Actual: len(lines),
			Limit:  height,
		})
	}

	for i, line := range lines {
		if strings.ContainsRune(line, '\t') {
			issues = append(issues, UITestIssue{
				Kind: IssueTabCharacter,
				Line: i + 1,
			})
		}
		if width > 0 {
			if w := ansi.StringWidth(line); w > width {
				issues = append(issues, UITestIssue{
					Kind:   IssueWidthOverflow,
					Line:   i + 1,
					Actual: w,
					Limit:  width,
				})
			}
		}
	}

	return issues
}
