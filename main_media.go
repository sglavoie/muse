package main

import (
	"errors"
	"log/slog"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/mpris"
	"github.com/sglavoie/muse/internal/tui"
)

// mediaControlsWanted reports whether this session should publish itself as
// an MPRIS player: on Linux, unless media_controls is off. A hosted session
// leaves the system media controls to its host app.
func mediaControlsWanted(goos string, enabled bool, o tuiOptions) bool {
	return goos == "linux" && enabled && !o.hosted()
}

// startMediaControls exports the MPRIS player and wires its close into
// shutdown. Media controls are a convenience, so any failure is logged and
// muse runs without them; it returns nil then, or when they are not wanted.
func startMediaControls(enabled bool, o tuiOptions, shutdownDeps *app.ShutdownDeps) *mpris.Service {
	if !mediaControlsWanted(runtime.GOOS, enabled, o) {
		return nil
	}
	svc, err := mpris.Start()
	if err != nil {
		if errors.Is(err, mpris.ErrUnavailable) {
			slog.Info("media controls unavailable", "err", err)
		} else {
			slog.Warn("media controls unavailable", "err", err)
		}
		return nil
	}
	slog.Info("media controls published", "bus_name", svc.BusName())
	shutdownDeps.CloseMediaControls = func() {
		if err := svc.Close(); err != nil {
			slog.Warn("shutdown: close media controls", "err", err)
		}
	}
	return svc
}

// mediaModelOptions makes svc hear the TUI's state events. It never hands the
// TUI a typed-nil observer.
func mediaModelOptions(svc *mpris.Service) []tui.Option {
	if svc == nil {
		return nil
	}
	return []tui.Option{tui.WithStateObserver(svc)}
}

// serveMediaControls lets media keys reach the TUI through send. It is a no-op
// when svc is nil.
func serveMediaControls(svc *mpris.Service, send func(tea.Msg)) {
	if svc != nil {
		svc.Serve(tui.ControlDispatch(send))
	}
}
