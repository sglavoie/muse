package library

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
)

func newRenderView(tracks []musiclib.Track) View {
	return View{
		Index:  buildIndex(tracks),
		Width:  80,
		Height: 24,
	}
}

func activateFilterQuery(v View, query string) View {
	v = v.ActivateFilter()
	for _, ch := range query {
		v = v.AppendFilterChar(ch)
	}
	return v
}

func renderLines(v View) []string {
	return strings.Split(v.Render(), "\n")
}

func renderTrackRows(t *testing.T, v View, wantRows int) []string {
	t.Helper()
	lines := renderLines(v)
	if len(lines) < 2+wantRows {
		t.Fatalf("expected at least %d lines (state strip + header + %d rows), got %d", 2+wantRows, wantRows, len(lines))
	}
	return lines[2:]
}

func renderTrackRow(t *testing.T, v View, row int) string {
	t.Helper()
	return renderTrackRows(t, v, row+1)[row]
}

func renderPlain(v View) string {
	return ansi.Strip(v.Render())
}

func requireANSIStyleSupport(t *testing.T, styled string) {
	t.Helper()
	if ansi.Strip(styled) == styled {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}
}

func assertRenderedLineWidths(t *testing.T, rendered string, width int) {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != width {
			t.Errorf("width=%d: row has %d display columns, want %d: %q", width, got, width, line)
		}
	}
}
