package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func renderStatusBarForTest(sb StatusBar, width int) string {
	return sb.render(width, styles.Styles{})
}

func renderStatusBarLines(t *testing.T, sb StatusBar, width int) []string {
	t.Helper()
	lines := strings.SplitN(renderStatusBarForTest(sb, width), "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in render output, got %d", len(lines))
	}
	return lines
}
