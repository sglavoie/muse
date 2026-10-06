package overlay

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// SeekTime is a timestamp input for the loaded track.
type SeekTime struct {
	Input        textinput.Model
	track        string
	Error        string
	CurrentSecs  float64
	DurationSecs float64
}

func NewSeekTime(track string) SeekTime {
	input := textinput.New()
	input.Placeholder = "12:30"
	input.CharLimit = 24
	input.SetWidth(24)
	return SeekTime{Input: input, track: track}
}

// ParseSeekTime accepts seconds, minutes:seconds, or hours:minutes:seconds.
func ParseSeekTime(value string) (float64, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("use seconds, m:ss, or h:mm:ss")
	}
	var total uint64
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return 0, fmt.Errorf("use seconds, m:ss, or h:mm:ss")
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || (i > 0 && n >= 60) {
			return 0, fmt.Errorf("invalid time; minutes and seconds after ':' must be below 60")
		}
		total = total*60 + n
	}
	return float64(total), nil
}

func (m SeekTime) Render(width, height int, s styles.Styles) string {
	inner := max(1, min(52, width-8))
	m.Input.SetWidth(min(24, inner))
	duration := "unknown"
	if m.DurationSecs > 0 {
		duration = text.FormatPositionSecs(m.DurationSecs)
	}
	timing := "Current: " + text.FormatPositionSecs(max(0, m.CurrentSecs)) + " · Duration: " + duration
	lines := []string{s.Heading.Render("Seek to time"), text.Truncate(m.track, inner),
		text.Truncate(timing, inner), m.Input.View(), "Seconds, m:ss, or h:mm:ss"}
	if m.Error != "" {
		lines = append(lines, s.Error.Render(ansi.Wrap(m.Error, inner, "")))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).
		Width(inner + 4).Render(strings.Join(lines, "\n"))
	return RenderCenteredLines(strings.Split(box, "\n"), "enter seek · esc cancel", width, height)
}
