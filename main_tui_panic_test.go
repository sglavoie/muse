package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

// panickingModel panics on its first Update so a real tea.Program exercises
// Bubble Tea's own recovery path.
type panickingModel struct{}

func (panickingModel) Init() tea.Cmd {
	return func() tea.Msg { return struct{}{} }
}

func (panickingModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	panic("injected TUI panic")
}

func (panickingModel) View() tea.View { return tea.NewView("") }

// TestReportProgramPanic_RealProgramPanicIsLoggedWithLogPath drives an actual
// tea.Program to a panic so the assertion covers the error Bubble Tea really
// returns, not a hand-built stand-in. A user filing a bug after a crash has a
// scrolled-away terminal, so the log file must carry the record.
func TestReportProgramPanic_RealProgramPanicIsLoggedWithLogPath(t *testing.T) {
	// Use the real file logger rather than a buffer: the point of the marker is
	// that it survives in config.LogPath() after the terminal has scrolled away,
	// so the assertion reads the actual file.
	stateDir := t.TempDir()
	orig := slog.Default()
	if err := config.InitLogging(stateDir, slog.LevelDebug); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}
	t.Cleanup(func() { slog.SetDefault(orig) })

	logPath := config.LogPath()
	if logPath == "" {
		t.Fatal("config.LogPath() is empty after InitLogging")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p := tea.NewProgram(panickingModel{},
		tea.WithContext(ctx),
		tea.WithInput(strings.NewReader("")),
		tea.WithOutput(&bytes.Buffer{}),
	)
	_, runErr := p.Run()
	if runErr == nil {
		t.Fatal("expected p.Run to return an error after the model panicked")
	}
	if !errors.Is(runErr, tea.ErrProgramPanic) {
		t.Fatalf("expected the run error to wrap tea.ErrProgramPanic; got %v", runErr)
	}

	var stderr bytes.Buffer
	if !reportProgramPanic(runErr, &stderr) {
		t.Fatal("reportProgramPanic returned false for a real program panic")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	logged := string(raw)
	if !strings.Contains(logged, "TUI panicked") {
		t.Errorf("expected a panic marker in %s; got: %s", logPath, logged)
	}
	if !strings.Contains(logged, tea.ErrProgramPanic.Error()) {
		t.Errorf("expected the underlying error in %s; got: %s", logPath, logged)
	}

	if !strings.Contains(stderr.String(), logPath) {
		t.Errorf("expected stderr to name the log path %q; got: %s", logPath, stderr.String())
	}
	if lines := strings.Count(strings.TrimSpace(stderr.String()), "\n"); lines != 0 {
		t.Errorf("expected a single stderr line; got %d extra newlines in: %s", lines, stderr.String())
	}
}

// TestReportProgramPanic_IgnoresOrdinaryErrors keeps the marker specific: a
// normal p.Run() failure must not be reported to the user as a crash.
func TestReportProgramPanic_IgnoresOrdinaryErrors(t *testing.T) {
	var logBuf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	var stderr bytes.Buffer
	if reportProgramPanic(errors.New("some other failure"), &stderr) {
		t.Error("reportProgramPanic must return false for a non-panic error")
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr output; got: %s", stderr.String())
	}
	if strings.Contains(logBuf.String(), "TUI panicked") {
		t.Errorf("expected no panic marker in the log; got: %s", logBuf.String())
	}
}
